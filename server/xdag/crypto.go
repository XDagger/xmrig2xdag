package xdag

/*
#cgo darwin LDFLAGS: -L${SRCDIR}/../../clib -lxdag_crypto_Darwin -L/usr/lib -lm
#cgo linux LDFLAGS: -L${SRCDIR}/../../clib -lxdag_crypto_Linux -L/usr/lib -lm
#cgo windows LDFLAGS: -L${SRCDIR}/../../clib -lxdag_crypto_Windows -L/usr/lib -lm
#include "../../clib/wrapper.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

func InitCrypto() C.int {
	return C.initCrypto()
}

func EncryptField(data unsafe.Pointer, sectorNo uint64) {
	C.encryptField(data, C.ulonglong(sectorNo))
}

func DecryptField(data unsafe.Pointer, sectorNo uint64) {
	C.decryptField(data, C.ulonglong(sectorNo))
}
