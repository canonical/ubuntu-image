# Ubuntu image builder

[![Build & Tests Status](https://travis-ci.org/canonical/ubuntu-image.svg?branch=master)](https://travis-ci.org/canonical/ubuntu-image)
[![codecov](https://codecov.io/gh/canonical/ubuntu-image/branch/master/graph/badge.svg)](https://codecov.io/gh/canonical/ubuntu-image)
[![Go Report Card](https://goreportcard.com/badge/github.com/canonical/ubuntu-image)](https://goreportcard.com/report/github.com/canonical/ubuntu-image)


This tool is used to build Ubuntu images.  Currently it only builds Snappy
images from a model assertion, but it will be generalized to build more
(eventually all) Ubuntu images.


# Requirements

Ubuntu 16.04 (Xenial Xerus) is the minimum platform requirement, but Ubuntu
18.04 (Bionic Beaver) or newer is recommended. All required third party packages are available in the
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
  (https://github.com/Canonical/ubuntu-image/blob/master/ubuntu-image.md)

The "gadget.yaml" specification has moved to [the snapcraft forum](https://forum.snapcraft.io/t/the-gadget-snap)
