#include <string>
#include <cstring>
#include "wrapper.h"
#include <cstdint>

static struct dfslib_crypt * g_crypt;

int initCrypto() {
    struct dfslib_string str;
    uint32_t sector[128];
    g_crypt = (dfslib_crypt *) malloc(sizeof(struct dfslib_crypt));
    if (!g_crypt) {
        return -1;
    }
    dfslib_crypt_set_password(g_crypt, dfslib_utf8_string(&str, MINERS_PWD, (uint32_t) strlen(MINERS_PWD)));
    for (int i = 0; i < 128; ++i) {
        sector[i] = SECTOR0_BASE + i * SECTOR0_OFFSET;
    }
    for (int i = 0; i < 128; ++i) {
        dfslib_crypt_set_sector0(g_crypt, sector);
        dfslib_encrypt_sector(g_crypt, sector, SECTOR0_BASE + i * SECTOR0_OFFSET);
    }
    return 0;
}


void encryptField(void *data, dfs64 sectorNo) {
    dfslib_encrypt_array(g_crypt, (uint32_t*)data, DATA_SIZE, sectorNo);
}

void decryptField(void *data, dfs64 sectorNo) {
    dfslib_uncrypt_array(g_crypt, (uint32_t*)data, DATA_SIZE, sectorNo);
}