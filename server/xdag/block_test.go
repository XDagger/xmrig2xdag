package xdag

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
)

func TestAddress2Hash1(t *testing.T) {
	address := "NrD1QgI1ORuQtjwTniSwr 8pTnS8Rgdu"
	hash, err := Address2hash(address)
	if err != nil {
		t.Error(err.Error())
	} else {
		fmt.Println(hex.EncodeToString(hash[:]))
	}
}

func TestAddress2Hash2(t *testing.T) {
	address := "YOUR_WALLET_ADDRESS"
	hash, err := Address2hash(address)
	if err != nil {
		t.Error(err.Error())
	} else {
		fmt.Println(hex.EncodeToString(hash[:]))
	}
}

func TestAddress2Hash3(t *testing.T) {
	address := "Pzza6lHK4GrZoEEBWh9wNbDjaNgJhq6V"
	hash, err := Address2hash(address)
	if err != nil {
		t.Error(err.Error())
	} else {
		fmt.Println(hex.EncodeToString(hash[:]))
	}
}

func TestHash2address(t *testing.T) {

	hash, err := hex.DecodeString("3f3cdaea51cae06ad9a041015a1f7035b0e368d80986ae950000000000000000")
	if err != nil {
		t.Error(err.Error())
	} else {
		str := Hash2address(hash[:])
		fmt.Println(str)
	}
}

func TestDifficulty(t *testing.T) {

	hashRate := 19.722698 // 19.722698M

	h := big.NewInt(int64(hashRate))

	h.Lsh(h, 26) // h.Lsh(h, 58) , 58-32

	//fmt.Println(h)
	//fmt.Printf("%032x\n", h)

	max128, _ := new(big.Int).SetString("ffffffffffffffffffffffffffffffff", 16)

	hash128 := new(big.Int).Div(max128, h)

	//s.Lsh(s, 32)  // 58-32

	fmt.Printf("%032x\n", hash128)

	hash64 := new(big.Int).Rsh(hash128, 64)

	max64, _ := new(big.Int).SetString("ffffffffffffffff", 16)

	diff := new(big.Int).Div(max64, hash64)

	fmt.Printf("%016x\n", diff)
	fmt.Println(diff)
}
