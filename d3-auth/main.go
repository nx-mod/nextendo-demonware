// d3-auth — serveur Demonware local pour Diablo III Switch (titre « crimson »).
//
// Le client (bdAuthSwitch, transport bdHTTPCurl) tape
//
//	https://crimson-switch-auth3.prod.demonware.net:<port>/auth/...
//
// puis bascule sur crimson-switch-lobby.*. Le NSO du jeu donne les hôtes, le
// gabarit d'URL « https://%s:%d/auth/ » et le vocabulaire du jeton
// (authToken=, foundAuthToken=, « Unknown player or bad authToken idSGame= »),
// mais PAS le format du corps.
//
// D'où ce service : on redirige les hôtes demonware vers nous dans le fichier
// hosts d'Atmosphere, et on journalise intégralement ce que la console envoie —
// sans jamais parler aux serveurs officiels. Le format de requête se déduit des
// captures, puis les réponses se remplissent ici.
//
// Tant que le format n'est pas connu, chaque requête est journalisée (méthode,
// chemin, en-têtes, corps en hexdump + ASCII) et reçoit une réponse vide 200.
// Le jeu la refusera : c'est attendu, on cherche d'abord à voir sa requête.
package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Taille attendue du client_ticket décodé. Le client la reçoit par sa vtable,
// donc elle n'est pas lisible statiquement : on l'ajuste à l'essai. Le tampon de
// chaîne fait 205 octets, ce qui plafonne le décodé à ~152.
// Magic du ticket bdAuth, lu dans le NSO (rodata 0xF07FB8) : octets DE AD BD EF.
const ticketMagic uint32 = 0xEFBDADDE

var clientTicketLen = 128

// Tickets emis, relus par d3-lobby (meme defaut relatif : ../sessions).
var sessionsDir = envOr("D3_SESSIONS", "../sessions")

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

var reqNum atomic.Uint64

// hexdump rend un bloc binaire lisible : offset, 16 octets hex, colonne ASCII.
// Le corps bdByteBuffer est typé (bdInt32/bdUInt16/bdUInt64 apparaissent dans le
// binaire), donc les motifs se repèrent à l'œil dans cette colonne.
func hexdump(b []byte) string {
	var sb strings.Builder
	for i := 0; i < len(b); i += 16 {
		end := i + 16
		if end > len(b) {
			end = len(b)
		}
		fmt.Fprintf(&sb, "  %08X  ", i)
		for j := i; j < i+16; j++ {
			if j < end {
				fmt.Fprintf(&sb, "%02X ", b[j])
			} else {
				sb.WriteString("   ")
			}
			if j-i == 7 {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(" |")
		for j := i; j < end; j++ {
			c := b[j]
			if c >= 0x20 && c < 0x7F {
				sb.WriteByte(c)
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
	}
	return sb.String()
}

func main() {
	addr := envOr("D3AUTH_LISTEN", ":8460")
	cert := envOr("D3AUTH_CERT", "../../nextendo/localcerts/cert.pem")
	key := envOr("D3AUTH_KEY", "../../nextendo/localcerts/key.pem")
	dumpDir := envOr("D3AUTH_DUMP", "dumps")

	if err := os.MkdirAll(dumpDir, 0o755); err != nil {
		log.Fatalf("dump dir: %v", err)
	}
	if v := os.Getenv("D3AUTH_TICKET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 152 {
			clientTicketLen = n
		}
	}

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqNum.Add(1)
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		r.Body.Close()

		log.Printf("==== #%d %s %s%s (%d octets)", n, r.Method, r.Host, r.URL.RequestURI(), len(body))
		for k, v := range r.Header {
			log.Printf("     %s: %s", k, strings.Join(v, ", "))
		}
		if len(body) > 0 {
			log.Printf("body:\n%s", hexdump(body))
			// Le corps brut est gardé sur disque : c'est lui qui sert à
			// reconstituer le format, l'hexdump n'est qu'un confort de lecture.
			name := fmt.Sprintf("%03d_%s_%s.bin", n, r.Method,
				strings.NewReplacer("/", "_", "?", "_", "&", "_", ":", "_").Replace(strings.TrimPrefix(r.URL.RequestURI(), "/")))
			if err := os.WriteFile(filepath.Join(dumpDir, name), body, 0o644); err != nil {
				log.Printf("     [dump] %v", err)
			}
		}

		// --- réponse d'authentification ---------------------------------
		// Forme déduite du parseur dans le NSO (0xBE30C0) :
		//   code          doit valoir 700 (0x2BC), sinon la tâche échoue
		//   iv_seed       entier, relu par le client
		//   client_ticket base64 -> exactement N octets (N vient de la vtable,
		//                 d'où D3AUTH_TICKET pour l'ajuster à l'essai)
		//   server_ticket base64 -> exactement 128 octets, recopiés tels quels
		//                 dans l'objet auth puis transmis au lobby. Opaque pour
		//                 le client : son contenu ne regarde que notre lobby.
		var req struct {
			AuthTask string `json:"auth_task"`
			IVSeed   string `json:"iv_seed"`
			TitleID  string `json:"title_id"`
			Identity string `json:"identity"`
		}
		_ = json.Unmarshal(body, &req)

		// Le client lit les 4 premiers octets du ticket et les compare au magic
		// 0xEFBDADDE (octets DE AD BD EF), résolu via une relocation
		// R_AARCH64_RELATIVE vers 0xF07FB8. S'ils correspondent, la branche de
		// déchiffrement est sautée et le ticket est lu en clair — c'est ce qui
		// permet un serveur autonome, sans clé ni patch du client.
		//
		// Disposition lue dans parse_ticket (0xBFCF30), 128 octets pile :
		//   +0 u32 magic | +4 u8 | +5 u32 | +9 u32 | +13 u32
		//   +17 u64 | +25 u64 | +33 [64] clé de session | +97 [24] | +121 [3] | +124 [4]
		clientTicket := make([]byte, 128)
		binary.LittleEndian.PutUint32(clientTicket[0:], ticketMagic)
		clientTicket[4] = 1
		binary.LittleEndian.PutUint32(clientTicket[5:], 5745)        // title id
		binary.LittleEndian.PutUint32(clientTicket[9:], 0)           // reserve
		binary.LittleEndian.PutUint32(clientTicket[13:], uint32(time.Now().Unix()))
		binary.LittleEndian.PutUint64(clientTicket[17:], 1)          // user id
		binary.LittleEndian.PutUint64(clientTicket[25:], uint64(time.Now().Unix()+86400))
		if _, err := rand.Read(clientTicket[33:97]); err != nil {    // clé de session
			log.Printf("     [rand] %v", err)
		}
		// +97 [24] : la cle du lobby. La connexion lobby (0xBFC8F0) copie un
		// bloc de config dont les octets 0x88..0xA0 deviennent conn+0x100, la
		// cle qui signe la poignee de main. Elle vient du champ de 24 octets du
		// ticket parse ; on y met les 24 premiers octets de la cle de session
		// pour que les deux lectures possibles donnent la meme valeur.
		copy(clientTicket[97:121], clientTicket[33:57])

		// Opaque pour le client : il le recopie tel quel et le transmet au lobby
		// dans le 0x82. C'est notre lobby qui le relit, donc on y met tout.
		serverTicket := append([]byte{}, clientTicket...)

		// Le lobby retrouve la cle de la session en essayant les tickets emis
		// recemment : c'est le tag du 0x82 qui designe le bon.
		if err := os.MkdirAll(sessionsDir, 0o755); err == nil {
			name := fmt.Sprintf("%d_%x.tkt", time.Now().Unix(), clientTicket[33:37])
			if err := os.WriteFile(filepath.Join(sessionsDir, name), clientTicket, 0o644); err != nil {
				log.Printf("     [session] %v", err)
			}
		}

		ivSeed := req.IVSeed
		if ivSeed == "" {
			ivSeed = "0"
		}

		// auth_task est lu AVANT code et validé par vtable[0x40] : sans lui la
		// réponse est rejetée avant même que code soit regardé (retour 735).
		//
		// Ce n'est PAS un écho : bdAuth apparie requête et réponse en n -> n+1.
		// Le hook posé dans d3hack l'a dit noir sur blanc — la console envoie 78
		// et attend 79 :
		//     [d3auth] parse ret=735 expected_task=79 client_ticket_len=128
		authTask := "79"
		if n, err := strconv.Atoi(req.AuthTask); err == nil {
			authTask = strconv.Itoa(n + 1)
		}

		// Le client encode TOUS ses entiers en chaînes JSON ("auth_task": "78",
		// "title_id": "5745") : on répond dans le même style, un entier nu ne
		// serait probablement pas relu par le même parseur.
		// Le decodeur du jeu utilise l'alphabet URL-safe (...0123456789-_ vu dans
		// rodata), pas le standard : un + ou / rendrait le decodage faux et la
		// longueur differente de 128, ce qui retombe sur le meme 735.
		// L'ORDRE compte : une map Go sérialise par clé triée, ce qui plaçait
		// client_ticket avant code. Le client lit dans un ordre fixe — auth_task,
		// code, iv_seed, client_ticket, server_ticket — et son parseur est
		// séquentiel. Une struct conserve l'ordre de déclaration.
		// extra_data : champ qui nous manquait, et la vraie raison des 735 en
		// boucle alors que les cinq autres champs etaient parfaits.
		//
		// vtable[0x50] (0xBE1440) le lit comme une CHAINE, la reparse comme du
		// JSON, puis y cherche nso_subscription_status qu'il lit en u16 :
		//
		//   if (has_member(extra, "nso_subscription_status")) {
		//       if (!get_u16(...)) ret = 0; else ret = 1;   // ecrase le reste
		//   }
		//
		// Un nso_subscription_status lu correctement suffit donc a faire
		// retourner vrai. C'est aussi la reponse a « il faut apparaitre en
		// ligne » : l'etat d'abonnement fait partie de la reponse d'auth, donc
		// il est entierement de notre ressort — aucun patch du jeu, aucun NSO
		// reel interroge.
		extra, _ := json.Marshal(map[string]string{
			"nso_subscription_status": "1",
		})

		resp := struct {
			AuthTask     string `json:"auth_task"`
			Code         string `json:"code"`
			IVSeed       string `json:"iv_seed"`
			ClientTicket string `json:"client_ticket"`
			ServerTicket string `json:"server_ticket"`
			ExtraData    string `json:"extra_data"`
		}{
			AuthTask:     authTask,
			Code:         "700",
			IVSeed:       ivSeed,
			ClientTicket: base64.StdEncoding.EncodeToString(clientTicket),
			ServerTicket: base64.StdEncoding.EncodeToString(serverTicket),
			ExtraData:    string(extra),
		}
		out, _ := json.Marshal(resp)

		log.Printf("     -> 200 code=700 auth_task=%s ticket=128B magic=0x%08X", authTask, ticketMagic)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: h,
		// Le client est un curl embarqué de 2018 : ne pas exiger TLS 1.2+.
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS10},
		ReadHeaderTimeout: 15 * time.Second,
	}
	log.Printf("[d3-auth] écoute %s (cert=%s) dumps=%s", addr, cert, dumpDir)
	log.Fatal(srv.ListenAndServeTLS(cert, key))
}
