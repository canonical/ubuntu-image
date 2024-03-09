# Contributing to ubuntu-image

This document covers the following topics:

* [Setting up](#setting-up)
* [Building](#building)
* [Testing](#testing)
* [Release process](#release-process)


## Setting up

Ubuntu 18.04 LTS or later is recommended for ubuntu-image development.
Usually, the [latest LTS](https://releases.ubuntu.com/) would be the best choice.

Go 1.21 [(or later)](https://go.dev/dl/) is required to build ubuntu-image.


## Building

1. Ensure golang >= 1.21 is installed.
2. Clone this repository in a directory where you have read-write permissions, such as your home directory: 
```sh
cd ~/
git clone https://github.com/canonical/ubuntu-image.git`
```
3. Change into the newly cloned repository:
```sh
cd ubuntu-image
```
4. Run:
```sh
go build -o . ./...
```

The newly compiled executable `ubuntu-image` will be created in the current directory. 


## Testing

We value good tests, so when you fix a bug or add a new feature we highly encourage you to add tests.

To run the test suite locally, install all the build dependencies named in the `debian/control` file. Run the following command from the root directory of the repository:
```sh
sudo apt build-dep ./
```

Alternatively, install the packages named in the `Build-Depends` field.


### Running unit-tests

To run the various tests that we have to ensure a high quality source just run:
```sh
go test -timeout 0 github.com/canonical/ubuntu-image/...
```

### Running integration tests

#### Downloading spread framework

To run the integration tests locally via QEMU, you need the latest version of the [spread](https://github.com/snapcore/spread) framework. You can get spread, QEMU, and the build tools to build QEMU images with:
```sh
sudo apt update && sudo apt install -y qemu-kvm autopkgtest
curl https://storage.googleapis.com/snapd-spread-tests/spread/spread-amd64.tar.gz | tar -xz -C <target-directory>
```

* `<target-directory>` can be any directory that is listed in `$PATH`, as it is assumed further in the guidelines of this document. You may consider creating a dedicated directory and adding it to `$PATH`, or you may choose to use one of the conventional Linux directories (e.g. `/usr/local/bin`)

#### Building spread VM images

To run the spread tests via QEMU you need to create VM images in the
`~/.spread/qemu` directory:
```sh
mkdir -p ~/.spread/qemu
cd ~/.spread/qemu
```

Assuming you are building on Ubuntu 18.04 LTS ([Bionic Beaver](https://releases.ubuntu.com/18.04/)) (or later), run the following to build a 64-bit Ubuntu 16.04 LTS (or later):
```sh
autopkgtest-buildvm-ubuntu-cloud -r <release-short-name>
mv autopkgtest-<release-short-name>-amd64.img ubuntu-<release-version>-64.img  
```

For the correct values of `<release-short-name>` and `<release-version>`, please refer to the official list of [Ubuntu releases](https://wiki.ubuntu.com/Releases). 

* `<release-short-name>` is the first word in the release's full name, 
e.g. for "Bionic Beaver" it is `bionic`.

If you are running Ubuntu 16.04 LTS, use `adt-buildvm-ubuntu-cloud` instead of `autopkgtest-buildvm-ubuntu-cloud` (the latter replaced the former in 18.04):
```sh
adt-buildvm-ubuntu-cloud -r xenial
mv adt-<release-name>-amd64-cloud.img ubuntu-<release-version>-64.img
```

#### Downloading spread VM images

Alternatively, instead of building the QEMU images manually, you can download pre-built and somewhat maintained images from [spread.zygoon.pl](https://spread.zygoon.pl/). The images will need to be extracted with `gunzip` and placed into `~/.spread/qemu` as above.

* An image for Ubuntu Core 20 that is pre-built for KVM can be downloaded from [here](https://cdimage.ubuntu.com/ubuntu-core/20/stable/current/ubuntu-core-20-amd64.img.xz).


#### Running spread with QEMU

Finally, you can run the spread tests for Ubuntu 18.04 LTS 64-bit with:
```sh
spread -v qemu:ubuntu-18.04-64
```

* To run for a different system, replace `ubuntu-18.04-64` with a different system name, which should be a basename of the [built](#building-spread-vm-images) or [downloaded](#downloading-spread-vm-images) Ubuntu image file.

For quick reuse you can use:
```sh
spread -reuse qemu:ubuntu-18.04-64
```

It will print how to reuse the systems. Make sure to use `export REUSE_PROJECT=1` in your environment too.

* Spread tests can be exercised on Ubuntu Core 20, but need UEFI. UEFI support with QEMU backend of spread requires a BIOS from the [OVMF](https://wiki.ubuntu.com/UEFI/OVMF) package, which can be installed with `sudo apt install ovmf`.


### Maintaining tests helpers

Spread tests rely on [snapd-testing-tools](https://github.com/snapcore/snapd-testing-tools). To update the subtree of this project, run:
```sh
git subtree pull --prefix tests/lib/external/snapd-testing-tools/ https://github.com/snapcore/snapd-testing-tools.git main --squash
```


## Release process

ubuntu-image is released as a snap on the [Snap Store](https://snapcraft.io/ubuntu-image).

When changes are merged in the `main` branch, a new snap is automatically built and pushed to the `latest/edge` channel.

When we think we have a "stable enough" version that we don't want to break with future merges in `main`, we promote it to `latest/candidate`. 

To do so:
1. Create a new branch.
2. Update the version in `snapcraft.yaml`.
3. Add a changelog entry to `debian/changelog`.
4. Commit, create a version tag, and push.
5. Merge.
6. Wait for the build to appear in `latest/edge`.
7. Promote the build to `latest/candidate`.

After a couple of weeks, if our "early adopters" are happy and we did not break any build or if we did not spot any major fixes, we promote this version from `latest/candidate` to `latest/stable`.

This way, our users can choose between:

- The `latest/edge` channel update as soon as we merge changes.
- The `latest/candidate` channel with new features/bugfixes but with potentially some newly introduced bug. This channel would be good to let users test requested features.
- The `latest/stable` channel that should hopefully contain a rather "bug-free" version because it was tested in more various situations.