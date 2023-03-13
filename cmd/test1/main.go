package main

import (
	"log"
	"os"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
)

func main() {
	h, err := hexutil.Decode(os.Args[1])
	if err != nil {
		log.Fatal(err)
		return
	}
	sk := new(bls.SecretKey).FromBEndian(h)
	a := bls.PublicKeyFromSecretKey(sk)
	log.Println(hexutil.Bytes(a.Compress()))
}
