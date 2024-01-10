# Hacking on ubuntu-image

## Setting up

### Supported Ubuntu distributions

Ubuntu 18.04 LTS or later is recommended for `ubuntu-image` development.
Usually, the latest LTS would be the best choice.

### Supported Go version

Go 1.21 (or later) is required to build `ubuntu-image`.


### Getting the ubuntu-image sources

The easiest way to get the source for `ubuntu-image` is to clone the GitHub repository
in a directory where you have read-write permissions, such as your home
directory.

    cd ~/
    git clone https://github.com/Canonical/ubuntu-image.git
    cd ubuntu-image

This will allow you to build and test `ubuntu-image`.


## Testing

We value good tests, so when you fix a bug or add a new feature we highly
encourage you to add tests.

### Running unit-tests

To run the various tests that we have to ensure a high quality source just run:

```bash
go test -timeout 0 github.com/canonical/ubuntu-image/...
```

### Running integration tests

#### Downloading spread framework

To run the integration tests locally via QEMU, you need the latest version of
the [spread](https://github.com/snapcore/spread) framework. 
You can get spread, QEMU, and the build tools to build QEMU images with:

    $ sudo apt update && sudo apt install -y qemu-kvm autopkgtest
    $ curl https://storage.googleapis.com/snapd-spread-tests/spread/spread-amd64.tar.gz | tar -xz -C <target-directory>

> `<target-directory>` can be any directory that is listed in `$PATH`, 
as it is assumed further in the guidelines of this document. 
You may consider creating a dedicated directory and adding it to `$PATH`, 
or you may choose to use one of the conventional Linux directories (e.g. `/usr/local/bin`)

#### Building spread VM images

To run the spread tests via QEMU you need to create VM images in the
`~/.spread/qemu` directory:

    $ mkdir -p ~/.spread/qemu
    $ cd ~/.spread/qemu

Assuming you are building on Ubuntu 18.04 LTS ([Bionic Beaver](https://releases.ubuntu.com/18.04/)) 
(or later), run the following to build a 64-bit Ubuntu 16.04 LTS (or later):

    $ autopkgtest-buildvm-ubuntu-cloud -r <release-short-name>
    $ mv autopkgtest-<release-short-name>-amd64.img ubuntu-<release-version>-64.img  

For the correct values of `<release-short-name>` and `<release-version>`, please refer
to the official list of [Ubuntu releases](https://wiki.ubuntu.com/Releases). 

> `<release-short-name>` is the first word in the release's full name, 
e.g. for "Bionic Beaver" it is `bionic`.

If you are running Ubuntu 16.04 LTS, use 
`adt-buildvm-ubuntu-cloud` instead of `autopkgtest-buildvm-ubuntu-cloud` (the
latter replaced the former in 18.04):

    $ adt-buildvm-ubuntu-cloud -r xenial
    $ mv adt-<release-name>-amd64-cloud.img ubuntu-<release-version>-64.img

#### Downloading spread VM images

Alternatively, instead of building the QEMU images manually, you can download
pre-built and somewhat maintained images from 
[spread.zygoon.pl](https://spread.zygoon.pl/). The images will need to be extracted 
with `gunzip` and placed into `~/.spread/qemu` as above.

> An image for Ubuntu Core 20 that is pre-built for KVM can be downloaded from 
[here](https://cdimage.ubuntu.com/ubuntu-core/20/stable/current/ubuntu-core-20-amd64.img.xz).

#### Running spread with QEMU

Finally, you can run the spread tests for Ubuntu 18.04 LTS 64-bit with:

    $ spread -v qemu:ubuntu-18.04-64

>To run for a different system, replace `ubuntu-18.04-64` with a different system
name, which should be a basename of the [built](#building-spread-vm-images) or 
[downloaded](#downloading-spread-vm-images) Ubuntu image file.

For quick reuse you can use:

    $ spread -reuse qemu:ubuntu-18.04-64

It will print how to reuse the systems. Make sure to use
`export REUSE_PROJECT=1` in your environment too.

> Spread tests can be exercised on Ubuntu Core 20, but need UEFI.
UEFI support with QEMU backend of spread requires a BIOS from the 
[OVMF](https://wiki.ubuntu.com/UEFI/OVMF) package, 
which can be installed with `sudo apt install ovmf`.

### Maintaining tests helpers

Spread tests rely on [snapd-testing-tools](https://github.com/snapcore/snapd-testing-tools). To update the subtree of this project, run:

```bash
git subtree pull --prefix tests/lib/external/snapd-testing-tools/ https://github.com/snapcore/snapd-testing-tools.git main --squash
```
