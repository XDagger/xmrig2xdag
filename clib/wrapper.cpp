#include "wrapper.h"

struct dfslib_crypt* initCrypto()
{
    struct dfslib_string str;
    uint32_t sector[128];
    struct dfslib_crypt * _crypt = (dfslib_crypt*)malloc(sizeof(struct dfslib_crypt));
    if(!_crypt)
    {
        return null;
    }
    dfslib_crypt_set_password(_crypt, dfslib_utf8_string(&str, MINERS_PWD, (uint32_t)strlen(MINERS_PWD)));
    for(int i = 0; i < 128; ++i)
    {
        sector[i] = SECTOR0_BASE + i * SECTOR0_OFFSET;
    }
    for(int i = 0; i < 128; ++i)
    {
        dfslib_crypt_set_sector0(_crypt, sector);
        dfslib_encrypt_sector(_crypt, sector, SECTOR0_BASE + i * SECTOR0_OFFSET);
    }
    return _crypt;
}