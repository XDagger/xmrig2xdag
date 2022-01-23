package xdag

/*
#cgo darwin LDFLAGS: -L${SRCDIR}/../../clib -lxdag_crypto_Darwin -L/usr/lib -lm
#cgo linux LDFLAGS: -L${SRCDIR}/../../clib -lxdag_crypto_Linux -L/usr/lib -lm
#cgo windows LDFLAGS: -L${SRCDIR}/../../clib -lxdag_cryptoe_Windows -L/usr/lib -lm
#include "../../clib/wrapper.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

func InitCrypto() unsafe.Pointer {
	return C.initCrypto()
}

func FreeCrypto(p unsafe.Pointer) {
	C.free(p)
}

func EncryptField(crypt unsafe.Pointer, data unsafe.Pointer, sectorNo uint64) {
	C.encryptField(crypt, data, C.ulonglong(sectorNo))
}

func DecryptField(crypt unsafe.Pointer, data unsafe.Pointer, sectorNo uint64) {
	C.decryptField(crypt, data, C.ulonglong(sectorNo))
}
