# XMRig2XDAG

XMRig2XDAG is a stratum proxy for  Monero (XMR) miners mining XDAG coins. 

XMRig2XDAG is translator between XMR stratum protocol and XDAG mining protocol. Written in Go.

XMRig can connect to XDAG mining pool through XMRig2XDAG proxy.

XDAG and XMR are using the same mining algorithm: RandomX.

XMRig2XDAG is Working with XMRig fork  https://github.com/swordlet/xmrig/tree/xdag.

# Guide

## start up
### proxy
./xmrig2xdag -c config.json

### miner
./xmrig -c config.json  (using administrator or root)

## config file
### xmrig2xdag config json file

    {
      "strport": 3232,                // port for xmrig to connect
      "url": "equal.xdag.org:13656",  // XDAG mining pool address and port
      "tls": false,                   // using SSL
      "debug": false,                 //  printing debug info
      "testnet": false                // using test net
    }

### xmrig config json file

    "pools": [
      {
        "algo": "rx/xdag",                     // mining XDAG
        "coin": null,
        "url": "stratum+tcp://127.0.0.1:3232", // xmrig2xdag address and port
        "user": "YOUR_WALLET_ADDRESS",         // your XDAG wallet address
        "pass": "your_miner_name",		   // naming your miner
        .....
      }
    ]

## Acknowledgement
https://github.com/xmrig/xmrig

https://github.com/xmrig/xmrig-proxy

https://github.com/trey-jones/stratum

https://github.com/trey-jones/xmrwasp