#include "dfstools/dfslib_crypt.h"

#define SECTOR0_BASE           0x1947f3acu
#define SECTOR0_OFFSET         0x82e9d1b5u
#define BLOCK_HEADER_WORD      0x3fca9e2bu
#define MINERS_PWD             "minersgonnamine"
#define DATA_SIZE              8 //(sizeof(struct xdag_field) / sizeof(uint32_t))
#define WORKERNAME_HEADER_WORD 0xf46b9853u

#ifdef __cplusplus
extern "C" {
#endif

extern void *initCrypto();
extern void encryptField(void *crypt, void *data, dfs64 sectorNo);
extern void decryptField(void *crypt, void *data, dfs64 sectorNo);

#ifdef __cplusplus
};
#endif