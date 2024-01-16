# Ubuntu image builder

![Build](https://github.com/canonical/ubuntu-image/actions/workflows/build-and-test.yml/badge.svg)
[![codecov](https://codecov.io/gh/canonical/ubuntu-image/branch/main/graph/badge.svg?token=F9jE9HKo1a)](https://codecov.io/gh/canonical/ubuntu-image)
[![Go Report Card](https://goreportcard.com/badge/github.com/canonical/ubuntu-image)](https://goreportcard.com/report/github.com/canonical/ubuntu-image)


This tool is used to build Ubuntu images.  Currently builds Ubuntu Core snap-based
images from model assertions and Ubuntu classic preinstalled images using image definition
files, but it will be generalized to build more (eventually all) Ubuntu images.


# Requirements

Ubuntu 18.04 (Bionic Beaver) is the minimum platform requirement, but Ubuntu
20.04 (Focal Fossa) or newer is recommended. All required third party packages are available in the
Ubuntu archive.

If you want to run the test suite locally, you should install all the build
dependencies named in the `debian/control` file.  The easiest way to do that
is to run::

    $ sudo apt build-dep ./

from the directory containing the `debian` subdirectory.  Alternatively of
course, you can just install the packages named in the `Build-Depends` field.


# Project details

* Project home: https://github.com/Canonical/ubuntu-image
* Report bugs at: https://bugs.launchpad.net/ubuntu-image
* Git clone: https://github.com/Canonical/ubuntu-image.git
* Manual page: man ubuntu-image
  (https://github.com/Canonical/ubuntu-image/blob/main/ubuntu-image.rst)

The "gadget.yaml" specification has moved to [the snapcraft forum](https://forum.snapcraft.io/t/gadget-snaps)

# Build Instructions

* Ensure golang >= 1.21 is installed
* Clone the git repository using `git clone https://github.com/canonical/ubuntu-image.git`
* `cd` into the newly cloned repository
* Run `go build -o . ./...`
* The newly compiled executable `ubuntu-image` will be created in the current directory


# Release process

ubuntu-image is released as a snap on [the snapstore](https://snapcraft.io/ubuntu-image).

When changes are merged in main, a new snap is automatically built and pushed to the `latest/edge` channel.

When we think we have a "stable enough" version that we don't want to break with future merges in main, we want to promote it to `latest/candidate`. To do so:
  - create a new branch
  - update the version in `snapcraft.yaml`, copyrights, changelog, etc.
  - commit, tag X.Y and push
  - merge
  - let it build and appear in `latest/edge`
  - promote it to `latest/candidate`

After a couple of weeks, if our "early adopters" are happy and we did not break any build or if we did not spot any major fix to do, promote this version from `latest/candidate` to `latest/stable`.

This way, our users can choose between:
- the `latest/edge` channel updated as soon as we merge changes.
- the `latest/candidate` channel with new features / bug fix but with potentially some newly introduced bug. This channel would be good to let users test requested features.
- the `latest/stable` channel that should hopefully contain a rather "bug free" version because it was tested in more various situations.