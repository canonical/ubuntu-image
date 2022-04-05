NONSNAP_GRUB_MODULES = \
btrfs \
hfsplus \
iso9660 \
part_apple \
part_msdos \
password_pbkdf2 \
zfs \
zfscrypt \
zfsinfo \
lvm \
mdraid09 \
mdraid1x \
raid5rec \
raid6rec \

# filtered list of modules included in the signed EFI grub image, excluding
# ones that we don't think are useful in snappy.
GRUB_MODULES = \
	all_video \
	biosdisk \
	boot \
	cat \
	chain \
	configfile \
	echo \
	ext2 \
	fat \
	font \
	gettext \
	gfxmenu \
	gfxterm \
	gfxterm_background \
	gzio \
	halt \
	jpeg \
	keystatus \
	loadenv \
	loopback \
	linux \
	memdisk \
	minicmd \
	normal \
	part_gpt \
	png \
	reboot \
	regexp \
	search \
	search_fs_uuid \
	search_fs_file \
	search_label \
	sleep \
	squash4 \
	test \
	true \
	video

all:
	dd if=$(SNAPCRAFT_STAGE)/usr/lib/grub/i386-pc/boot.img of=pc-boot.img bs=440 count=1
	/bin/echo -n -e '\x90\x90' | dd of=pc-boot.img seek=102 bs=1 conv=notrunc
	grub-mkimage -d $(SNAPCRAFT_STAGE)/usr/lib/grub/i386-pc/ -O i386-pc -o pc-core.img -p '(,gpt2)/EFI/ubuntu' $(GRUB_MODULES)
	dd if=/dev/urandom bs=4096 count=1 of=foobare.img
	dd if=/dev/urandom bs=4096 count=1 of=foowithoutfs.img
	dd if=/dev/urandom bs=1M count=10 of=foo.txt
	mkfs.ext4 foowithfs-v1.img 100m
	# The first sector of the core image requires an absolute pointer to the
	# second sector of the image.  Since this is always hard-coded, it means our
	# BIOS boot partition must be defined with an absolute offset.  The
	# particular value here is 2049, or 0x01 0x08 0x00 0x00 in little-endian.
	/bin/echo -n -e '\x01\x08\x00\x00' | dd of=pc-core.img seek=500 bs=1 conv=notrunc
	cp $(SNAPCRAFT_STAGE)/usr/lib/shim/shimx64.efi.dualsigned shim.efi.signed
	cp $(SNAPCRAFT_STAGE)/usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed grubx64.efi

install:
	install -m 644 pc-boot.img pc-core.img shim.efi.signed grubx64.efi foobare.img foowithoutfs.img foo.txt foowithfs-v1.img $(DESTDIR)/
	install -m 644 grub.conf $(DESTDIR)/
