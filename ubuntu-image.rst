==============
 ubuntu-image
==============

------------------------------
Generate a bootable disk image
------------------------------

:Authors:
    Barry Warsaw <barry@ubuntu.com>,
    Łukasz 'sil2100' Zemczak <lukasz.zemczak@ubuntu.com>
:Date: 2021-03-31
:Copyright: 2016-2021 Canonical Ltd.
:Version: 1.11
:Manual section: 1


SYNOPSIS
========

ubuntu-image snap [options] model.assertion

ubuntu-image classic [options] GADGET_TREE_URI


DESCRIPTION
===========

``ubuntu-image`` is a program for generating a variety of bootable disk
images.  It currently supports building snap_ based and classic preinstalled
Ubuntu images.

Snap-based images are built from a *model assertion*, which is a YAML_ file
describing a particular combination of core, kernel, and gadget snaps, along
with other declarations, signed with a digital signature asserting its
authenticity.  The assets defined in the model assertion uniquely describe the
device for which the image is built.

As part of the model assertion, a `gadget snap`_ is specified.  The gadget
contains a `gadget.yaml`_ file which contains the exact description of the
disk image's contents, in YAML format.  The ``gadget.yaml`` file describes
such things as the names of all the volumes to be produced [#]_, the
structures [#]_ within the volume, whether the volume contains a bootloader
and if so what kind of bootloader, etc.

Note that ``ubuntu-image`` communicates with the snap store using the ``snap
prepare-image`` subcommand.  The model assertion file is passed to ``snap
prepare-image`` which handles downloading the appropriate gadget and any extra
snaps.  See that command's documentation for additional details.

Classic images are built from a local `gadget tree`_ path.  The `gadget tree`_
is nothing more than a primed `gadget snap`_, containing a `gadget.yaml`_ file
in the meta directory and all the necessary bootloader gadget bits built.
For instance a `gadget tree`_ can be easily prepared by fetching a specially
tailored `gadget snap`_ source and running ``snapcraft prime`` on it, with the
resulting tree ending up in the ``prime/`` directory.

The actual rootfs for a classic image is created by ``live-build`` with
arguments passed as per the optional arguments to ``ubuntu-image``.  The
``livecd-rootfs`` configuration from the host system is used.


OPTIONS
=======

-h, --help
    Show the program's message and exit.

--version
    Show the program's version number and exit.


Snap command options
--------------------

These are the options for defining the contents of snap-based images.  Can only
be used when the ``ubuntu-image snap`` command is used.

model_assertion
    Path to the model assertion file.  This positional argument must be given
    for this mode of operation.

--snap SNAP
    Install an extra snap.  This is passed through to ``snap prepare-image``.
    The snap argument can include additional information about the channel
    and/or risk with the following syntax: ``<snap>=<channel|risk>``

--extra-snaps EXTRA_SNAPS
    **DEPRECATED** (Use ``--snap`` instead.) Extra snaps to install.  This is
    passed through to ``snap prepare-image``.

--cloud-init USER-DATA-FILE
    ``cloud-config`` data to be copied to the image.

-c CHANNEL, --channel CHANNEL
    The snap channel to use.

--disable-console-conf
    Disable console-conf on the resulting image.

--factory-image
    Hint that the image is meant to boot in a device factory.

Classic command options
-----------------------

These are the options for defining the contents of classic preinstalled Ubuntu
images.  Can only be used when the ``ubuntu-image classic`` command is used.

GADGET_TREE_URI
    An URI to the gadget tree to be used to build the image.  This positional
    argument must be given for this mode of operation.  Must be a local path.

-p PROJECT, --project PROJECT
    Project name to be passed on to ``livecd-rootfs``.

-s SUITE, --suite SUITE
    Distribution name to be passed on to ``livecd-rootfs``.

-a CPU-ARCHITECTURE, --arch CPU-ARCHITECTURE
    CPU architecture to be passed on to ``livecd-rootfs``.  Default value is
    the architecture of the host.

--subproject SUBPROJECT
    Sub-project name to be passed on ``livecd-rootfs``.

--subarch SUBARCH
    Sub-architecture to be passed on to ``livecd-rootfs``.

--with-proposed
    Defines if the image should be built with -proposed enabled.  This is
    passed through to ``livecd-rootfs``.

--extra-ppas EXTRA_PPAS
    Extra ppas to install. This is passed through to ``livecd-rootfs``.


Common options
--------------

There are two general operational modes to ``ubuntu-image``.  The usual mode
is to run the script giving the required model assertion file as a required
positional argument, generating a disk image file.  These options are useful
in this mode of operation.

The second mode of operation is provided for debugging and testing purposes.
It allows you to run the internal state machine step by step, and is described
in more detail below.

-d, --debug
    Enable debugging output.

-O DIRECTORY, --output-dir DIRECTORY
    Write generated disk image files to this directory.  The files will be
    named after the ``gadget.yaml`` volume names, with ``.img`` suffix
    appended.  If not given, the current working directory is used.  This
    option replaces, and cannot be used with, the deprecated ``--output``
    option.

-o FILENAME, --output FILENAME
    **DEPRECATED** (Use ``--output-dir`` instead.)  The generated disk image
    file.  If not given, the image will be put in a file called ``disk.img``
    in the working directory, in which case, you probably want to specify
    ``--workdir``.  If ``--workdir`` is not given, the image will be written
    to the current working directory.

-i SIZE, --image-size SIZE
    The size of the generated disk image files.  If this size is smaller than
    the minimum calculated size of the volume, a warning will be issued and
    ``--image-size`` will be ignored.  The value is the size in bytes, with
    allowable suffixes 'M' for MiB and 'G' for GiB.

    An extended syntax is supported for gadget.yaml files which specify
    multiple volumes (i.e. disk images).  In that case, a single ``SIZE``
    argument will be used for all the defined volumes, with the same rules for
    ignoring values which are too small.  You can specify the image size for a
    single volume using an indexing prefix on the ``SIZE`` parameter, where
    the index is either a volume name or an integer index starting at zero.
    For example, to set the image size only on the second volume, which might
    be called ``sdcard`` in the gadget.yaml, you could use: ``--image-size
    1:8G`` since the 1-th index names the second volume (volumes are
    0-indexed).  Or you could use ``--image-size sdcard:8G``.

    You can also specify multiple volume sizes by separating them with commas,
    and you can mix and match integer indexes and volume name indexes.  Thus,
    if the gadget.yaml named three volumes, and you wanted to set all three to
    different sizes, you could use ``--image-size 0:2G,sdcard:8G,eMMC:4G``.

    In the case of ambiguities, the size hint is ignored and the calculated
    size for the volume will be used instead.

--image-file-list FILENAME
    Print to ``FILENAME``, a list of the file system paths to all the disk
    images created by the command, if any.

--hooks-directory DIRECTORY
    Path or comma-separated list of paths of directories in which scripts for
    build-time hooks will be located.

--disk-info DISK-INFO-CONTENTS
    File to be used as .disk/info on the image's rootfs.  This file can
    contain useful information about the target image, like image
    identification data, system name, build timestamp etc.


State machine options
---------------------

.. caution:: The options described here are primarily for debugging and
   testing purposes and should not be considered part of the stable, public
   API.  State machine step numbers and names can change between releases.

``ubuntu-image`` internally runs a state machine to create the disk image.
These are some options for controlling this state machine.  Other than
``--workdir``, these options are mutually exclusive.  When ``--until`` or
``--thru`` is given, the state machine can be resumed later with ``--resume``,
but ``--workdir`` must be given in that case since the state is saved in a
``.ubuntu-image.pck`` file in the working directory.

-w DIRECTORY, --workdir DIRECTORY
    The working directory in which to download and unpack all the source files
    for the image.  This directory can exist or not, and it is not removed
    after this program exits.  If not given, a temporary working directory is
    used instead, which *is* deleted after this program exits.  Use
    ``--workdir`` if you want to be able to resume a partial state machine
    run.  As an added bonus, the ``gadget.yaml`` file is copied to the working
    directory after it's downloaded.

-u STEP, --until STEP
    Run the state machine until the given ``STEP``, non-inclusively.  ``STEP``
    can be the name of a state machine method, or a number indicating the
    ordinal of the step.

-t STEP, --thru STEP
    Run the state machine through the given ``STEP``, inclusively.  ``STEP``
    can be the name of a state machine method, or a number indicating the
    ordinal of the step.

-r, --resume
    Continue the state machine from the previously saved state.  It is an
    error if there is no previous state.


FILES
=====

gadget.yaml
    https://github.com/snapcore/snapd/wiki/Gadget-snap#gadget.yaml

model assertion
    https://developer.ubuntu.com/en/snappy/guides/prepare-image/

gadget tree (example)
    https://github.com/snapcore/pc-amd64-gadget

cloud-config
    https://help.ubuntu.com/community/CloudInit


ENVIRONMENT
===========

The following environment variables are recognized by ``ubuntu-image``.

``UBUNTU_IMAGE_SNAP_CMD``
    ``ubuntu-image`` calls ``snap prepare-image`` to communicate with the
    store, download the gadget, and unpack its contents.  Normally for the
    ``ubuntu-image`` deb, whatever ``snap`` command is first on your ``$PATH``
    is used, while for the classic snap, the bundled ``snap`` command is used.
    Set this environment variable to specify an alternative ``snap`` command
    which ``prepare-image`` is called on.

``UBUNTU_IMAGE_PRESERVE_UNPACK``
    When set, this names a directory for preserving a pristine copy of the
    unpacked gadget contents.  The directory must exist, and an ``unpack``
    directory will be created under this directory.  The full contents of the
    ``<workdir>/unpack`` directory after the ``snap prepare-image`` subcommand
    has run will be copied here.

``UBUNTU_IMAGE_LIVECD_ROOTFS_AUTO_PATH``
    ``ubuntu-image`` uses ``livecd-rootfs`` configuration files for its
    ``live-build`` runs.  If this variable is set, ``ubuntu-image`` will use
    the configuration files from the selected path for its auto configuration.
    Otherwise it will attempt to localize ``livecd-rootfs`` through a call to
    ``dpkg``.

``UBUNTU_IMAGE_QEMU_USER_STATIC_PATH``
    In case of classic image cross-compilation for a different architecture,
    ``ubuntu-image`` will attempt to use the qemu-user-static emulator with
    ``live-build``.  If set, ``ubuntu-image`` will use the selected path for
    the cross-compilation.  Otherwise it will attempt to find a matching
    emulator binary in the current ``$PATH``.

There are a few other environment variables used for building and testing
only.


HOOKS
=====

During image build at certain stages of the build process the user can execute
custom scripts modifying its contents or otherwise affecting the process
itself.  Whenever a hook is to be fired, the directories as listed in the
``--hooks-directory`` parameter are scanned for matching scripts.  There can be
multiple scripts for a specific hook defined.  The ``HookManager`` will first
look for executable files in ``<hookdir>/<name-of-the-hook>.d`` and execute
them in an alphanumerical order.  Finally the ``<hookdir>/<name-of-the-hook>``
file is executed if existing.

Hook scripts can have various additional data passed onto them through
environment variables depending on the hook being fired.

Currently supported hooks:

post-populate-rootfs
    Executed after the rootfs directory has been populated, allowing
    custom modification of its contents.  Added in version 1.2.  Environment
    variables present:

        ``UBUNTU_IMAGE_HOOK_ROOTFS``
            Includes the absolute path to the rootfs contents.


NOTES
=====

Sometimes, for various reasons, ``ubuntu-image`` may perform specific
workarounds that might require some explanation to understand the reasoning
behind them.

Classic swapfile manual unsparsing
----------------------------------

When building a classic image, if ``ubuntu-image`` notices the existence of a
``/swapfile`` on the image's rootfs, it will proactively attempt to unsparse
it.  The reason for that is that ``ubuntu-image`` assumes that the ``/swapfile``
file will be used as a swapfile on the target system, and due to undocumented
behavior of ``mkfs.ext4 -d`` large empty files are converted into sparse files
automatically during filesystem population.  This essentially makes such files
unusable as swapfiles.  So just in case, ``ubuntu-image`` does an in-place
``dd`` call of the hard-coded path swapfile to ensure it's no longer sparse.


SEE ALSO
========

snap(1)


FOOTNOTES
=========

.. [#] Volumes are roughly analogous to disk images.
.. [#] Structures define the layout of the volume, including partitions,
       Master Boot Records, or any other relevant content.


.. _snap: http://snapcraft.io/
.. _YAML: https://developer.ubuntu.com/en/snappy/guides/prepare-image/
.. _`gadget snap`: https://github.com/snapcore/snapd/wiki/Gadget-snap
.. _`gadget tree`: Example: https://github.com/snapcore/pc-amd64-gadget
.. _`gadget.yaml`: https://github.com/snapcore/snapd/wiki/Gadget-snap#gadget.yaml
