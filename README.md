# Ubuntu image builder

[![Build & Tests Status](https://travis-ci.org/canonical/ubuntu-image.svg?branch=master)](https://travis-ci.org/canonical/ubuntu-image)
[![codecov](https://codecov.io/gh/canonical/ubuntu-image/branch/master/graph/badge.svg)](https://codecov.io/gh/canonical/ubuntu-image)
[![Go Report Card](https://goreportcard.com/badge/github.com/canonical/ubuntu-image)](https://goreportcard.com/report/github.com/canonical/ubuntu-image)


This tool is used to build Ubuntu images.  Currently it only builds Snappy
images from a model assertion, but it will be generalized to build more
(eventually all) Ubuntu images.


# Requirements

Ubuntu 16.04 (Xenial Xerus) is the minimum platform requirement, but Ubuntu
18.04 (Bionic Beaver) or newer is recommended. WILLIAM TODO ADD NOTE ABOUT REQUIRED GOLANG VERSION.  All required third party packages are available in the
Ubuntu archive.

If you want to run the test suite locally, you should install all the build
dependencies named in the `debian/control` file.  The easiest way to do that
is to run::

    $ sudo apt build-dep ./

from the directory containing the `debian` subdirectory.  Alternatively of
course, you can just install the packages named in the `Build-Depends` field.


# License

``ubuntu-image`` is licensed under the terms of the GPLv3 and Copyright by
Canonical Ltd.

WILLIAM TODO ADD GOLANG LICENSE INFO


# Project details

* Project home: https://github.com/Canonical/ubuntu-image
* Report bugs at: https://bugs.launchpad.net/ubuntu-image
* Git clone: https://github.com/Canonical/ubuntu-image.git
* Documentation: TBD WILLIAM GENERATE GODOC
* Manual page: man ubuntu-image
  (https://github.com/Canonical/ubuntu-image/blob/master/ubuntu-image.md)

The ``gadget.yaml`` specification has moved to `the snapcraft forum`_.

.. _`the snapcraft forum`: https://forum.snapcraft.io/t/the-gadget-snap


# Developing

WILLIAM TODO THIS ALL NEEDS TO BE CHANGED
You'll need the `tox <https://pypi.python.org/pypi/tox>`__ tool to run the
test suite (see above for testing requirements).  You can run the full test
suite, including coverage and code quality tests via::

    $ tox

You can run individual tests like this::

    $ tox -e py37-nocov -- -P <pattern>

where *<pattern>* is a Python regular expression matching a test name, e.g.::

    $ tox -e py37-nocov -- -P test_smoke

Pull requests run the same test suite that archive promotion (i.e. -proposed
to release pocket) runs.  You can reproduce this locally by building the
source package (with ``gbp buildpackage -S``) and running::

    $ autopkgtest ubuntu-image_2.dsc -- schroot focal-amd64

with changes to the version number and Ubuntu distroseries as appropriate.
