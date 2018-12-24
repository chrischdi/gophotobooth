FROM scratch

ARG rootfs_tar
ARG qemu_binary

# add rootfs
ADD tmp/$rootfs_tar /
# enable qemu emulation
COPY tmp/$qemu_binary /usr/bin/

# do stuff
RUN pacman-key --init && pacman-key --populate archlinuxarm
RUN pacman -Syuu --noconfirm
RUN pacman --noconfirm -S \
    xf86-video-fbdev \
    xorg-server \
    xorg-xinit \
    xorg-xset \
    xterm
# xorg-xset: to disable blanking
# xorg-apps

# build stuff
RUN pacman --noconfirm -S \
    base-devel \
    git \
    pkgconf \
    rsync

# gophotobooth specific libraries and tools
RUN pacman --noconfirm -S \
    gtk3 \
    imagemagick \
    libgphoto2
# gtk3: for gui
# imagemagick: for creating images in dummy driver
# libgphoto2: for triggering camera

# additional stuff
RUN pacman --noconfirm -S \
    htop \
    vim

# enable i2c, spi and gpio
RUN groupadd i2c -r \
    && usermod -a -G i2c alarm
RUN groupadd gpio -r \
    && usermod -a -G gpio alarm
RUN groupadd spi -r \
    && usermod -a -G spi alarm
COPY image/udev.d/99-raspberrypi.rules /etc/udev/rules.d/99-raspberrypi.rules

# autologin
RUN mkdir -p /etc/systemd/system/getty@tty1.service.d/
COPY image/getty-override.conf /etc/systemd/system/getty@tty1.service.d/override.conf

COPY image/xinitrc /etc/X11/xinit/xinitrc

# rotate display
RUN echo "display_rotate=2" >> /boot/config.txt
# enforce DMR
RUN echo "hdmi_group=2" >> /boot/config.txt
# enforce 1280x800 resolution
RUN echo "hdmi_mode=27" >> /boot/config.txt

# enable mounting usb disk
RUN pacman --noconfirm -S \
  ntfs-3g
RUN echo "/dev/sda1 /mnt ntfs defaults,users 0 0" >> /etc/fstab

# configure alarm user
USER alarm
COPY image/alarm/bashrc /home/alarm/.bashrc

USER root

# wifi setup
RUN ln -s /usr/lib/systemd/system/wpa_supplicant@.service /etc/systemd/system/multi-user.target.wants/wpa_supplicant@wlan0.service
COPY image/wpa_supplicant-wlan0.conf /etc/wpa_supplicant/wpa_supplicant-wlan0.conf
COPY image/00-wireless-dhcp.network /etc/systemd/network/00-wireless-dhcp.network

# copy binaries and stuff
COPY bin/ /usr/local/bin/

# cleanup
RUN pacman --noconfirm -Scc
RUN rm /usr/bin/$qemu_binary
