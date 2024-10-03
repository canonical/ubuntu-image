# Contributing to ubuntu-image

This document covers the following topics:

* [Setting up](#setting-up)
* [Building](#building)
* [Testing](#testing)
* [Release process](#release-process)


## Setting up

Ubuntu 22.04 LTS or later is recommended for ubuntu-image development.
Usually, the [latest LTS](https://releases.ubuntu.com/) would be the best choice.

Go 1.21 [(or later)](https://snapcraft.io/go) is required to build ubuntu-image.


## Building

1. Clone this repository in a directory where you have read-write permissions, such as your home directory: 
```
cd ~/
git clone https://github.com/canonical/ubuntu-image.git`
```
2. Change into the newly cloned repository:
```
cd ubuntu-image
```
3. Run:
```
CGO_ENABLED=0 go build -o . ./...
```

The newly compiled executable `ubuntu-image` will be created in the current directory. 


## Testing

We value good tests, so when you fix a bug or add a new feature we highly encourage you to add tests.

To run the test suite locally, install all the build dependencies named in the `debian/control` file. Run the following command from the root directory of the repository:
```
sudo apt build-dep ./
```

Alternatively, install the packages named in the `Build-Depends` field.


### Running unit-tests

To run the various tests that we have to ensure a high quality source:
```
go test -timeout 0 ./...
```

### Running integration tests

#### Downloading Spread framework

+To run the integration tests locally via QEMU, you need the latest version of the [Spread](https://github.com/snapcore/spread) framework. You can get Spread, QEMU, and the build tools to build QEMU images with:
```
sudo apt update && sudo apt install -y qemu-kvm autopkgtest
curl https://storage.googleapis.com/snapd-spread-tests/spread/spread-amd64.tar.gz | tar -xz -C <target-directory>
```

* `<target-directory>` can be any directory that is listed in `$PATH`, as it is assumed further in the guidelines of this document. Either create a dedicated directory and add it to `$PATH`, or use one of the conventional Linux directories (e.g. `/usr/local/bin`).

#### Building Spread VM images

To run the Spread tests via QEMU, create VM images in the
`~/.spread/qemu` directory:
```
mkdir -p ~/.spread/qemu
cd ~/.spread/qemu
```

Run the following to build a 64-bit Ubuntu 22.04 LTS (or later):
```
autopkgtest-buildvm-ubuntu-cloud -r <release-short-name>
mv autopkgtest-<release-short-name>-amd64.img ubuntu-<release-version>-64.img  
```

For the correct values of `<release-short-name>` and `<release-version>`, see the official list of [Ubuntu releases](https://wiki.ubuntu.com/Releases). 

* `<release-short-name>` is the first word in the release's full name, 
e.g. for "Noble Numbat" it is `noble`.

#### Downloading Spread VM images

Alternatively, instead of building the QEMU images manually, you can download unofficial pre-built images from [spread.zygoon.pl](https://spread.zygoon.pl/). Extract these images with `gunzip` and place them into `~/.spread/qemu` as above.

* To download an image for the latest Ubuntu Core that is pre-built for KVM, see [cdimage.ubuntu.com/ubuntu-core](https://cdimage.ubuntu.com/ubuntu-core/).


#### Running Spread with QEMU

Finally, you can run the Spread tests for Ubuntu 22.04 LTS 64-bit with:
```
spread -v qemu:ubuntu-22.04-64
```

* To run for a different system, replace `ubuntu-22.04-64` with a different system name, which should be a basename of the [built](#building-spread-vm-images) or [downloaded](#downloading-spread-vm-images) Ubuntu image file.

For quick reuse, run:
```
spread -reuse qemu:ubuntu-22.04-64
```

It prints how to reuse the systems. Run `export REUSE_PROJECT=1` in your environment, too.

* Spread tests can be exercised on Ubuntu Core 20 and higher but need UEFI. UEFI support with a QEMU backend for Spread requires a BIOS from the [OVMF](https://wiki.ubuntu.com/UEFI/OVMF) package, which can be installed with `sudo apt install ovmf`.


### Maintaining test helpers

Spread tests rely on [snapd-testing-tools](https://github.com/snapcore/snapd-testing-tools). To update the subtree of this project, run:
```
git subtree pull --prefix tests/lib/external/snapd-testing-tools/ https://github.com/snapcore/snapd-testing-tools.git main --squash
```


## mkfs configuration

ubuntu-image embeds in the snap different configurations to generate filesystems compatible with different series of ubuntu (and specifically different releases of `mkfs`). The snap building process should detect if one of these configurations is out of date and should fail. In this case, update the configurations by running the following from the project root directory:
```
# First install dependencies if needed
sudo apt install ubuntu-dev-tools

# Run the configuration updater
./tools/collect-mkfs-confs.sh
```

Then check the configurations and the `./mkfs/db` file were updated. Commit the resulting changes.


## Release process

ubuntu-image is released as a snap on the [Snap Store](https://snapcraft.io/ubuntu-image).

When changes are merged in the `main` branch, a new snap is automatically built and pushed to the `latest/edge` channel by Launchpad.

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


## Rebuilding stable snaps

To fix vulnerabilities in dependencies pulled when building the snap, we have to rebuild the snap.

To do so:
1. Get the git tag associated to the published snap
2. Update the `Source` on the `ubuntu-image-rebuild` snap recipe on Launchpad with the tag.
3. Request a build.
4. (optional) Check the build was triggered from the same commit as the snap you want to replace
5. Promote the build from `latest/beta` to `latest/stable`.
