# Build binary

On target system: 
```
make build
```

Cross-compile for armv7:
```
make rpi-image-arm cross-arm-deps build-arm
```
This command will:
* build a image for raspberry pi using docker, based on archlinuxarm using qemu emulation targetting armv7
* download cross-compiler
* unpack image and cross-compiler
* cross-compile the binary
