package xdag

import (
	"encoding/hex"
	"fmt"
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
