# Ubuntu image builder

![Build](https://github.com/canonical/ubuntu-image/actions/workflows/build-and-test.yml/badge.svg)
[![codecov](https://codecov.io/gh/canonical/ubuntu-image/branch/main/graph/badge.svg?token=F9jE9HKo1a)](https://codecov.io/gh/canonical/ubuntu-image)
[![Go Report Card](https://goreportcard.com/badge/github.com/canonical/ubuntu-image)](https://goreportcard.com/report/github.com/canonical/ubuntu-image)

This tool is used to build Ubuntu images:

* Ubuntu Core snap-based images from model assertions
* Ubuntu classic preinstalled images using image definition

Future versions should be generalized to build more (eventually all) Ubuntu images.


## Requirements

Ubuntu 18.04 (Bionic Beaver) is the minimum platform requirement, but Ubuntu 20.04 (Focal Fossa) or newer is recommended. All required third party packages are available in the Ubuntu archive.

To run the test suite locally, install all the build dependencies named in the `debian/control` file. Run the following command from the root directory of the repository:

    $ sudo apt build-dep ./

Alternatively, install the packages named in the `Build-Depends` field.


## Project details

* Project home: https://github.com/Canonical/ubuntu-image
* Report bugs at: https://bugs.launchpad.net/ubuntu-image
* Git clone: https://github.com/Canonical/ubuntu-image.git
* Manual page: [`man ubuntu-image`](https://github.com/Canonical/ubuntu-image/blob/main/ubuntu-image.rst)

The `gadget.yaml` specification has moved to [the snapcraft forum](https://forum.snapcraft.io/t/gadget-snaps).


## Build instructions

1. Ensure golang >= 1.21 is installed.
2. Clone the Git repository using `git clone https://github.com/canonical/ubuntu-image.git`.
3. `cd` into the newly cloned repository.
4. Run `go build -o . ./...`.
5. The newly compiled executable `ubuntu-image` is created in the current directory.


## Release process

ubuntu-image is released as a snap on the [Snap Store](https://snapcraft.io/ubuntu-image).

When changes are merged in the `main` branch, a new snap is automatically built and pushed to the `latest/edge` channel.

When we think we have a "stable enough" version that we don't want to break with future merges in main, we promote it to `latest/candidate`. To do so:

1. Create a new branch.
2. Update the version in `snapcraft.yaml`.
3. Add a changelog entry to `debian/changelog`.
4. Commit, create a version tag, and push.
5. Merge.
6. Wait for the build to appear in `latest/edge`.
7. Promote the build to `latest/candidate`.

After a couple of weeks, if our "early adopters" are happy and we did not break any build or if we did not spot any major fix to do, promote this version from `latest/candidate` to `latest/stable`.

This way, our users can choose between:

- The `latest/edge` channel updated as soon as we merge changes.
- The `latest/candidate` channel with new features/bugfixes but with potentially some newly introduced bug. This channel would be good to let users test requested features.
- The `latest/stable` channel that should hopefully contain a rather "bug-free" version because it was tested in more various situations.
