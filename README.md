# XMRig2XDAG

XMRig2XDAG is a stratum proxy for  Monero (XMR) miners mining XDAG coins. 

XMRig2XDAG is translator between XMR stratum protocol and XDAG mining protocol. Written in Go.

XMRig can connect to XDAG mining pool through XMRig2XDAG proxy.

XDAG and XMR are using the same mining algorithm: RandomX.

XMRig2XDAG is Working with XMRig fork  https://github.com/swordlet/xmrig/tree/xdag.

# Build

## under clib folder

    $ cmake .
    $ make
## under server folder

    $ go mod tidy
    $ CGO_ENABLED=1 go build

# Guide
start up command line and configure file

## start up
start up  xmrig2xdag proxy and xmrig miner

### proxy
./xmrig2xdag -c config.json

### miner
./xmrig -c config.json  (using administrator or root)

## config file
xmrig2xdag proxy and xmrig config file

### xmrig2xdag config json file

    {
      "strport": 3232,                // port for xmrig to connect
      "url": "equal.xdag.org:13656",  // XDAG mining pool address and port
      "tls": false,                   // using SSL
      "debug": false,                 //  printing debug info
      "testnet": false,               // using test net
      "socks5": ""                    // SOCKS5 proxy address:port 
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

## Arm or Android Mobile

CPU must above Snpadragon 810 for Android

install termux on Android and install scrcpy on PC for better interaction

in Arm computer or Android Mobile command line:

    apt update && apt upgrade &&

    apt install -y git wget proot build-essential cmake libmicrohttpd &&

    git clone -b xdag https://github.com/swordlet/xmrig --depth 1 &&

    mkdir xmrig/build &&

    cd xmrig/build &&

    cmake -DWITH_HWLOC=OFF .. &&

    make -j10

edit config.json following the guide

    ./xmrig -c config.json

connect XDAG pool through xmr2xdag proxy running in local network

## Acknowledgement
https://github.com/xmrig/xmrig

https://github.com/xmrig/xmrig-proxy

https://github.com/trey-jones/stratum

https://github.com/trey-jones/xmrwasp