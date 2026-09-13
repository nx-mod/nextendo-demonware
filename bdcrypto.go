package main

// Primitives cryptographiques du lobby Demonware, reproduites a l'identique de
// ce que fait le client (LibTomCrypt dans le NSO, SHA-1 partout).
//
//	0xBDB5F0  hmacSHA1(cle 20 octets, donnees)
//	0xBDB730  hkdfExpandSHA1(cle, etiquette, n)  — T1 = HMAC(k, lbl||01),
//	          Ti = HMAC(k, T(i-1)||lbl||i), sortie tronquee a n
//	0xBDC9D0  sha1(donnees)
//	+0x230/+0x238 : AES-128-CBC (objet de 0x6B0 octets, init avec 16 octets)

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
)

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func hmacSHA1(key, data []byte) []byte {
	m := hmac.New(sha1.New, key)
	m.Write(data)
	return m.Sum(nil)
}

// hkdfExpandSHA1 suit la boucle de 0xBDB730 : le premier bloc ne prefixe pas
// de T(0) vide, le compteur est un octet qui commence a 1.
func hkdfExpandSHA1(key, label []byte, n int) []byte {
	out := make([]byte, 0, n+sha1.Size)
	var prev []byte
	for i := byte(1); len(out) < n; i++ {
		buf := make([]byte, 0, len(prev)+len(label)+1)
		buf = append(buf, prev...)
		buf = append(buf, label...)
		buf = append(buf, i)
		prev = hmacSHA1(key, buf)
		out = append(out, prev...)
	}
	return out[:n]
}

// sessionKeys est ce que 0xBFBB80 derive a la fin de la poignee de main.
type sessionKeys struct {
	clientTag [8]byte // 8 derniers octets du message 0x82
	serverChk [8]byte // a renvoyer dans 0x83 (compare a conn+0x224)
	c2sMAC    []byte  // conn+0x240, 20 octets
	s2cMAC    []byte  // conn+0x254, 20 octets
	c2sAES    []byte  // objet conn+0x230, 16 octets
	s2cAES    []byte  // objet conn+0x238, 16 octets
}

// deriveKeys : secret = HMAC-SHA1(cle=SHA1(transcript), msg=cle24), puis
// CLIENTCHAL (16) et BDDATA (72).
func deriveKeys(transcript, key24 []byte) sessionKeys {
	digest := sha1.Sum(transcript)
	secret := hmacSHA1(digest[:], key24)

	chal := hkdfExpandSHA1(secret, []byte("CLIENTCHAL"), 16)
	data := hkdfExpandSHA1(secret, []byte("BDDATA"), 72)

	var k sessionKeys
	copy(k.clientTag[:], chal[0:8])
	copy(k.serverChk[:], chal[8:16])
	k.c2sMAC = append([]byte{}, data[0:20]...)
	k.s2cMAC = append([]byte{}, data[20:40]...)
	k.c2sAES = append([]byte{}, data[40:56]...)
	k.s2cAES = append([]byte{}, data[56:72]...)
	return k
}

// wrapKey24 est la variante du drapeau conn+0x208 : la cle du ticket passe par
// HKDF avec la cle publique DER comme etiquette.
func wrapKey24(key24 []byte) []byte {
	return hkdfExpandSHA1(key24, bdPublicKeyDER, 24)
}

func aesCBCDecrypt(key, iv, ct []byte) ([]byte, error) {
	if len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return nil, errors.New("longueur chiffree invalide")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	pt := make([]byte, len(ct))
	cipher.NewCBCDecrypter(b, iv).CryptBlocks(pt, ct)
	return pt, nil
}

func aesCBCEncrypt(key, iv, pt []byte) ([]byte, error) {
	if len(pt)%aes.BlockSize != 0 {
		return nil, errors.New("clair non aligne")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	ct := make([]byte, len(pt))
	cipher.NewCBCEncrypter(b, iv).CryptBlocks(ct, pt)
	return ct, nil
}
