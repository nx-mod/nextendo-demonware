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
// serveSTUN repond aux sondes NAT Demonware sur UDP 3074 (hotes
// stun.{us,eu,jp,au}.demonware.net). Ce n'est pas du STUN RFC 5389 : trois
// octets d'en-tete bruts « type | version | bourrage », puis des champs bruts.
// Format repris du serveur STUN de project-bo4/shield-development (meme
// generation du SDK Demonware) et confirme par les sondes de la console :
//
//	1e 03 00       type 30, decouverte d'IP  -> 31 | 02 | 00 | ip[4] | port u16
//	14 02 00 00    type 20, decouverte NAT   -> 21 | 02 | 00 | ip[4] | port u16 | ipServeur[4] | portServeur u16
//
// L'IP s'ecrit en ordre reseau, le port en u16 petit-boutiste. Sans ces
// reponses le type de NAT reste inconnu et le jeu ne propose que des parties
// locales.
func serveSTUN(port string, dumpDir string) {
	pc, err := net.ListenPacket("udp", ":"+port)
	if err != nil {
		log.Printf("[stun] udp %s indisponible: %v", port, err)
		return
	}
	serverIP := net.ParseIP(envOr("D3_SERVER_IP", "192.168.137.1")).To4()
	log.Printf("[stun] a l'ecoute UDP %s (ip serveur annoncee %s)", port, serverIP)

	buf := make([]byte, 2048)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil || n < 3 {
			continue
		}
		ua, ok := addr.(*net.UDPAddr)
		if !ok || ua.IP.To4() == nil {
			continue
		}
		sig := fmt.Sprintf("%X", buf[:n])
		stunSeenMu.Lock()
		cnt := stunSeen[sig]
		stunSeen[sig] = cnt + 1
		stunSeenMu.Unlock()

		// Traversee NAT (paquets de 29 octets, format documente par
		// Protarium-Network/bo2-wiiu-demonware) :
		//	type | u16 version | id[10] | hmac[4] | adresseSource[6] | adresseDest[6]
		// 0x0A : un joueur qui rejoint demande a etre presente a l'hote. On
		// renvoie le paquet OCTET POUR OCTET a la destination avec le type 0x0B
		// (INTRO) ; le HMAC couvre les adresses et seul le demandeur le verifie,
		// donc rien d'autre ne doit changer. L'hote repond 0x0C directement au
		// demandeur, ce qui ouvre la connexion P2P. 0x0E : keepalive, sans reponse.
		if n == 29 && (buf[0] == 0x0A || buf[0] == 0x0E) {
			if buf[0] == 0x0E {
				continue
			}
			dst := buf[23:29]
			dstIP := net.IPv4(dst[0], dst[1], dst[2], dst[3])
			dstPort := int(binary.LittleEndian.Uint16(dst[4:6]))
			if dstPort == 0 || dstIP.Equal(net.IPv4(0, 255, 0, 255)) {
				log.Printf("[nat] 0x0A de %s sans destination: %X", addr, buf[:n])
				continue
			}
			intro := append([]byte{0x0B}, buf[1:n]...)
			to := &net.UDPAddr{IP: dstIP, Port: dstPort}
			if _, err := pc.WriteTo(intro, to); err != nil {
				log.Printf("[nat] INTRO vers %s: %v", to, err)
			} else {
				log.Printf("[nat] INTRO %s -> %s (id=%X)", addr, to, buf[3:13])
			}
			continue
		}

		resp := []byte{}
		switch buf[0] {
		case 30:
			resp = append(resp, 31, 2, 0)
			resp = append(resp, ua.IP.To4()...)
			resp = binary.LittleEndian.AppendUint16(resp, uint16(ua.Port))
		case 20:
			resp = append(resp, 21, 2, 0)
			resp = append(resp, ua.IP.To4()...)
			resp = binary.LittleEndian.AppendUint16(resp, uint16(ua.Port))
			resp = append(resp, serverIP...)
			resp = binary.LittleEndian.AppendUint16(resp, 3074)
		default:
			if cnt < 2 {
				log.Printf("[stun] paquet inconnu de %s: %s", addr, hex.EncodeToString(buf[:n]))
				_ = os.WriteFile(filepath.Join(dumpDir, fmt.Sprintf("stun_unknown_%d.bin", connNum.Add(1))), buf[:n], 0o644)
			}
			continue
		}
		if _, err := pc.WriteTo(resp, addr); err != nil {
			log.Printf("[stun] reponse a %s: %v", addr, err)
		} else if cnt < 2 {
			log.Printf("[stun] %s type %d -> %X", addr, buf[0], resp)
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
