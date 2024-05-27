# ubuntu-image: build Ubuntu images

[![ubuntu-image](https://snapcraft.io/ubuntu-image/badge.svg)](https://snapcraft.io/ubuntu-image)
![Build](https://github.com/canonical/ubuntu-image/actions/workflows/build-and-test.yml/badge.svg?branch=main)
[![codecov](https://codecov.io/gh/canonical/ubuntu-image/branch/main/graph/badge.svg?token=F9jE9HKo1a)](https://codecov.io/gh/canonical/ubuntu-image)
[![Go Report Card](https://goreportcard.com/badge/github.com/canonical/ubuntu-image)](https://goreportcard.com/report/github.com/canonical/ubuntu-image)

ubuntu-image is a tool used for generating bootable images. You can use it to build Ubuntu images such as:

- Snap-based Ubuntu Core images from model assertions

- Classical preinstalled Ubuntu images using image definitions

The future versions of this tool will be more generalized, allowing users to build a wider range of Ubuntu images, including ISO/installer.

## Getting started

### Requirements

* Ubuntu 20.04 (Focal Fossa) or newer (recommended: Ubuntu 22.04 (Jammy Jellyfish))

* Ability to install snaps ([SnapStore: ubuntu-image](https://snapcraft.io/ubuntu-image))

### Quickstart

See [Build your first Ubuntu Core image](https://ubuntu.com/core/docs/build-an-image) for instructions on how to use ubuntu-image to build an Ubuntu core image on a **Raspberry Pi**.

> [!IMPORTANT] 
> `ubuntu-image` requires **elevated permissions**. Run it with **root** privileges or using `sudo`.

## Building images

ubuntu-image offers two basic sub-commands for building snap-based and classical images.

### Building snap-based images

To build a snap-based image with ubuntu-image, you need a [model assertion](https://ubuntu.com/core/docs/reference/assertions/model). A model assertion is a YAML file that describes a particular combination of core, kernel, and gadget snaps, along with other declarations, signed with a digital signature asserting its authenticity. The `ubuntu-image` command only requires the path to this model assertion to build snap-based images.

To build snap-based images with `ubuntu-image`, use the following command:

```
ubuntu-image snap model.assertion
```

See [Build your first Ubuntu Core image](https://ubuntu.com/core/docs/build-an-image) for more information on building snap-based images using ubuntu-image. To build an image with custom snaps, see [Build an image with custom snaps](https://ubuntu.com/core/docs/custom-images).

### Building classical images

Classical images are built from image definitions, which are YAML files. The image definition YAML file specifies the various configurations required to build a classical image, including the path to the `gadget.yaml` file. See [Image Definition](internal/imagedefinition/README.rst) for the detailed specification of what is supported in the image definition YAML file.

To build classical images with ubuntu-image, use the following command:

```
ubuntu-image classic image_definition.yaml
```

## Building and testing ubuntu-image

See [Contributing to ubuntu-image](/CONTRIBUTING.md) for instructions on how to set up, build, and test ubuntu-image in development mode.

## License

The ubuntu-image project is licensed under [GNU General Public License v3.0](/LICENSE).

## Contributing to ubuntu-image

To learn how to contribute to the ubuntu-image project, see [Contributing to ubuntu-image](/CONTRIBUTING.md).

## Project details

* Project home: https://github.com/Canonical/ubuntu-image
* Report bugs at: https://bugs.launchpad.net/ubuntu-image
* Git clone: https://github.com/Canonical/ubuntu-image.git
* Reference page: [`ubuntu-image` syntax and options](https://canonical-subiquity.readthedocs-hosted.com/en/latest/reference/ubuntu-image.html)
* Building a gadget snap: [Building a gadget snap](https://ubuntu.com/core/docs/gadget-building)
* Gadget tree: [pc-gadget](https://github.com/snapcore/pc-gadget)
* `gadget.yaml` specification: [Gadget snaps](https://forum.snapcraft.io/t/gadget-snaps)
