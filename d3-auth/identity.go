package main

// Identite du joueur lue dans la requete d'authentification.
//
// extra_data (chaine JSON) porte « username » (le surnom Nintendo) et « token »
// (id_token NSA). Le champ « nnex » du jeton vaut « nx2.<b64>.<sig> » ou <b64>
// decode donne « <PID Nextendo>.<surnom>.<horodatage> ». Ce PID est l'identifiant
// que Citron utilise pour ses amis (vu : su6ur6an = 1800011760 = 0x6B49FFF0).

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
)

type identity struct {
	Username string `json:"username"`
	PID      uint64 `json:"pid"`
	Sub      string `json:"sub"`
}

func b64any(s string) []byte {
	s = strings.TrimRight(s, "=")
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b
	}
	return nil
}

func playerIdentity(body []byte) identity {
	var id identity
	var req struct {
		ExtraData string `json:"extra_data"`
	}
	if json.Unmarshal(body, &req) != nil {
		return id
	}
	var extra struct {
		Username string `json:"username"`
		Token    string `json:"token"`
	}
	if json.Unmarshal([]byte(req.ExtraData), &extra) != nil {
		return id
	}
	id.Username = extra.Username

	parts := strings.Split(extra.Token, ".")
	if len(parts) < 2 {
		return id
	}
	var claims struct {
		Sub  string `json:"sub"`
		Nnex string `json:"nnex"`
	}
	if json.Unmarshal(b64any(parts[1]), &claims) != nil {
		return id
	}
	id.Sub = claims.Sub
	nn := strings.TrimPrefix(claims.Nnex, "nx2.")
	if i := strings.IndexByte(nn, '.'); i > 0 {
		nn = nn[:i]
	}
	if dec := string(b64any(nn)); dec != "" {
		f := strings.SplitN(dec, ".", 3)
		if pid, err := strconv.ParseUint(f[0], 10, 64); err == nil {
			id.PID = pid
		}
		if id.Username == "" && len(f) > 1 {
			id.Username = f[1]
		}
	}
	return id
}
