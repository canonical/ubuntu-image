# ubuntu-image: build Ubuntu images

[![ubuntu-image](https://snapcraft.io/ubuntu-image/badge.svg)](https://snapcraft.io/ubuntu-image)
![Build](https://github.com/canonical/ubuntu-image/actions/workflows/build-and-test.yml/badge.svg)
[![codecov](https://codecov.io/gh/canonical/ubuntu-image/branch/main/graph/badge.svg?token=F9jE9HKo1a)](https://codecov.io/gh/canonical/ubuntu-image)
[![Go Report Card](https://goreportcard.com/badge/github.com/canonical/ubuntu-image)](https://goreportcard.com/report/github.com/canonical/ubuntu-image)


ubuntu-image is used to build the following Ubuntu images:

* Snap-based Ubuntu Core images from model assertions
* Classical preinstalled Ubuntu images using image definition

Future versions should be generalized to build more (eventually all) Ubuntu images.


## Requirements

Ubuntu 18.04 (Bionic Beaver) is the minimum platform requirement, but Ubuntu 20.04 (Focal Fossa) or newer is recommended. All required third party packages are available in the Ubuntu archive.


## Project details

* Project home: https://github.com/Canonical/ubuntu-image
* Report bugs at: https://bugs.launchpad.net/ubuntu-image
* Git clone: https://github.com/Canonical/ubuntu-image.git
* Reference page: [`ubuntu-image` syntax and options](https://canonical-subiquity.readthedocs-hosted.com/en/latest/reference/ubuntu-image.html)
* `gadget.yaml` specification: [Gadget snaps](https://forum.snapcraft.io/t/gadget-snaps)


## Building & testing

For instructions on building and testing ubuntu-image, see [CONTRIBUTING.md](./CONTRIBUTING.md).

