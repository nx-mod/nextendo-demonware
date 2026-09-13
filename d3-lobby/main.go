// d3-lobby — serveur lobby Demonware local pour Diablo III Switch.
//
// L'auth (d3-auth) parle HTTPS/JSON sur :443 et passe par sni-router. Le lobby,
// lui, est une connexion TCP directe sur 3074 : un hello brut de 28 octets, puis
// des trames « u32 longueur | drapeau | type | corps ». La poignee de main
// (0x81/0x82/0x83) et le canal chiffre qui suit sont decrits dans handshake.go.
//
// Les sondes NAT du jeu arrivent en UDP sur le meme port : serveSTUN.
package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

var connNum atomic.Uint64

var (
	stunSeen   = map[string]int{}
	stunSeenMu sync.Mutex
)
// serveSTUN repond aux sondes NAT sur UDP. Les hotes stun.*.demonware.net
// etaient absents du fichier hosts de la console, donc elle les envoyait
// directement chez Activision (45 paquets vers 185.34.107.128:3074 en une
// session). Redirige en local, il faut quelqu'un pour repondre.
//
// On journalise d'abord le paquet recu : Demonware utilise le port 3074 mais
// pas forcement du STUN RFC 5389 strict, et le hexdump le dira. Si l'en-tete
// ressemble a du STUN (type 0x0001, magic cookie 0x2112A442), on renvoie une
// Binding Success Response avec XOR-MAPPED-ADDRESS, ce qui est ce qu'un client
// attend pour apprendre son adresse publique.
func serveSTUN(port string, dumpDir string) {
	pc, err := net.ListenPacket("udp", ":"+port)
	if err != nil {
		log.Printf("[stun] udp %s indisponible: %v", port, err)
		return
	}
	log.Printf("[stun] a l'ecoute UDP %s", port)

	buf := make([]byte, 2048)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			continue
		}
		k := connNum.Add(1)
		// Les sondes arrivent par paires ~8 fois par seconde : on ne journalise
		// que les premieres de chaque forme, sinon elles noient le reste. Les
		// formes vues : "1e 03 00" et "14 02 00 00" (pas du STUN RFC 5389).
		sig := fmt.Sprintf("%X", buf[:n])
		stunSeenMu.Lock()
		cnt := stunSeen[sig]
		stunSeen[sig] = cnt + 1
		stunSeenMu.Unlock()
		if cnt < 2 {
			log.Printf("==== #%d STUN udp=%s from=%s %d octets:\n%s", k, port, addr, n, hex.Dump(buf[:n]))
		} else if cnt == 200 {
			log.Printf("[stun] forme %s vue 200 fois, journalisation coupee", sig)
		}
		if n > 0 {
			name := fmt.Sprintf("stun_%03d_udp%s.bin", k, port)
			_ = os.WriteFile(filepath.Join(dumpDir, name), buf[:n], 0o644)
		}

		if n < 20 {
			continue
		}
		msgType := binary.BigEndian.Uint16(buf[0:])
		cookie := binary.BigEndian.Uint32(buf[4:])
		if msgType != 0x0001 || cookie != 0x2112A442 {
			log.Printf("#%d pas du STUN standard (type=0x%04X cookie=0x%08X) — pas de reponse", k, msgType, cookie)
			continue
		}

		ua, ok := addr.(*net.UDPAddr)
		if !ok {
			continue
		}
		ip4 := ua.IP.To4()
		if ip4 == nil {
			continue
		}

		// Binding Success Response (0x0101) + XOR-MAPPED-ADDRESS (0x0020).
		resp := make([]byte, 0, 32)
		resp = append(resp, 0x01, 0x01) // type
		resp = append(resp, 0x00, 0x0C) // longueur du corps
		resp = append(resp, buf[4:20]...)

		xport := uint16(ua.Port) ^ uint16(cookie>>16)
		xip := make([]byte, 4)
		binary.BigEndian.PutUint32(xip, binary.BigEndian.Uint32(ip4)^cookie)

		resp = append(resp, 0x00, 0x20, 0x00, 0x08, 0x00, 0x01)
		resp = binary.BigEndian.AppendUint16(resp, xport)
		resp = append(resp, xip...)

		if _, err := pc.WriteTo(resp, addr); err != nil {
			log.Printf("#%d reponse STUN: %v", k, err)
		} else {
			log.Printf("#%d -> STUN binding response, mapped=%s:%d", k, ua.IP, ua.Port)
		}
	}
}

func main() {
	// 3074 est le port historique du stack bd (partagé avec Xbox Live) ; les
	// voisins sont là parce que le port réel n'est pas déductible du binaire.
	ports := strings.Split(envOr("D3LOBBY_PORTS", "3074,3075,3076,3077,3078,3079,3080"), ",")
	dumpDir := envOr("D3LOBBY_DUMP", "dumps")
	if err := os.MkdirAll(dumpDir, 0o755); err != nil {
		log.Fatalf("dump dir: %v", err)
	}
	// Tickets emis par d3-auth : la cle de 24 octets du lobby en sort.
	sessDir := envOr("D3_SESSIONS", "../sessions")
	// Fichiers « publisher » (saison, evenements, liste noire) generes par d3-pubfiles.
	pubDir := envOr("D3_PUBFILES", "../d3-pubfiles/files")

	opened := 0
	for _, p := range ports {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		ln, err := net.Listen("tcp", ":"+p)
		if err != nil {
			log.Printf("[d3-lobby] port %s indisponible: %v", p, err)
			continue
		}
		opened++
		go func(ln net.Listener, port string) {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				lc := &lobbyConn{c: c, n: connNum.Add(1), dumpDir: dumpDir, sessDir: sessDir, pubDir: pubDir}
				go lc.run()
			}
		}(ln, p)
	}

	if opened == 0 {
		log.Fatal("[d3-lobby] aucun port ouvert")
	}
	// Les sondes NAT arrivent en UDP sur le meme port que le lobby.
	go serveSTUN("3074", dumpDir)

	log.Printf("[d3-lobby] à l'écoute sur %d port(s): %s — dumps=%s", opened, strings.Join(ports, ","), dumpDir)
	select {}
}
