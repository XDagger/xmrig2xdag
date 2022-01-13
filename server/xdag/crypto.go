package xdag

/*
#include "dfstools/wrapper.h"
*/
import "C"

func InitCrypto() interface{} {
	return C.initCrypto()
}
