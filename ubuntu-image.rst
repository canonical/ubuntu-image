==============
 ubuntu-image
==============

------------------------------
Generate a bootable disk image
------------------------------

:Authors:
    Barry Warsaw <barry@ubuntu.com>,
    Łukasz 'sil2100' Zemczak <lukasz.zemczak@ubuntu.com>,
    William 'jawn-smith' Wilson <william.wilson@canonical.com>,
    Paul Mars <paul.mars@canonical.com>
:Date: 2021-10-21
:Copyright: 2016-2021 Canonical Ltd.
:Version: 3.0
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

--cloud-init USER-DATA-FILE
    ``cloud-config`` data to be copied to the image.

--disable-console-conf
    Disable console-conf on the resulting image.

--factory-image
    Hint that the image is meant to boot in a device factory.

--validation=<ignore|enforce>
    Control whether validations should be ignored or enforced.

--snap SNAP
    Install an extra snap.  This is passed through to ``snap prepare-image``.
    The snap argument can include additional information about the channel
    and/or risk with the following syntax: ``<snap>=<channel|risk>``. Note
    that this flag will cause an error if the model assertion has a grade
    higher than dangerous

--revision SNAP_NAME:REVISION
    Install a specific revision of a snap, rather than the latest available
    in a particular channel. The snap specified with SNAP_NAME must be
    included either in the model assertion or as an argument to --snap. If
    both a revision and channel are provided, the revision specified will be
    installed in the image, and updates will come from the specified channel

Classic command options
-----------------------

These are the options for defining the contents of classic preinstalled Ubuntu
images.  Can only be used when the ``ubuntu-image classic`` command is used.

image_definition
    Path to the image definition file. This file defines all of the
    customization required when building your image. This positional
    argument must be given for this mode of operation.


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

--verbose
    Enable verbose output.

--quiet
    Only print error messages. Suppress all other output.

-O DIRECTORY, --output-dir DIRECTORY
    Write generated disk image files to this directory.  The files will be
    named after the ``gadget.yaml`` volume names, with ``.img`` suffix
    appended.  If not given, the value of the --workdir flag is used if
    --workdir is specified.  If neither --output-dir or --workdir is used,
    the image(s) will be placed in the current working directory.  This
    option replaces, and cannot be used with, the deprecated ``--output``
    option.

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

--disk-info DISK-INFO-CONTENTS
    File to be used as .disk/info on the image's rootfs.  This file can
    contain useful information about the target image, like image
    identification data, system name, build timestamp etc.

-c CHANNEL, --channel CHANNEL
    The default snap channel to use while preseeding the image.

--sector-size SIZE
    When creating the disk image file, use the given sector size.  This
    can be either 512 or 4096 (4k sector size), defaulting to 512.


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
``ubuntu-image.json`` file in the working directory.

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
    is the name of a state machine method. The list of all steps can be
    found in the STEPS section of this document.

-t STEP, --thru STEP
    Run the state machine through the given ``STEP``, inclusively.  ``STEP``
    is the name of a state machine method. The list of all steps can be
    found in the STEPS section of this document.

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
    https://github.com/snapcore/pc-gadget

cloud-config
    https://help.ubuntu.com/community/CloudInit


ENVIRONMENT
===========

The following environment variables are recognized by ``ubuntu-image``.

``UBUNTU_IMAGE_PRESERVE_UNPACK``
    When set, this names a directory for preserving a pristine copy of the
    unpacked gadget contents.  The directory must exist, and an ``unpack``
    directory will be created under this directory.  The full contents of the
    ``<workdir>/unpack`` directory after the ``snap prepare-image`` subcommand
    has run will be copied here.

``UBUNTU_IMAGE_QEMU_USER_STATIC_PATH``
    In case of classic image cross-compilation for a different architecture,
    ``ubuntu-image`` will attempt to use the qemu-user-static emulator with
    ``live-build``.  If set, ``ubuntu-image`` will use the selected path for
    the cross-compilation.  Otherwise it will attempt to find a matching
    emulator binary in the current ``$PATH``.

There are a few other environment variables used for building and testing
only.


STEPS
=====

The names of steps that can be used with --until and --thru for each image
type are listed below

Classic image steps
-------------------

State machines are dynamically created for classic image builds based on
the contents of the image definition. The list of all possible states
is as follows:

#. make_temporary_directories
#. parse_image_definition
#. calculate_states
#. build_gadget_tree
#. prepare_gadget_tree
#. load_gadget_yaml
#. create_chroot
#. germinate
#. add_extra_ppas
#. install_packages
#. verify_artifact_names
#. customize_cloud_init
#. customize_fstab
#. manual_customization
#. preseed_image
#. clean_rootfs
#. populate_rootfs_contents
#. generate_disk_info
#. calculate_rootfs_size
#. populate_bootfs_contents
#. populate_prepare_partitions
#. make_disk
#. generate_manifest
#. finish

To check the steps that are going to be used for a specific image
definition file, use the ``--print-states`` flag.

Snap image steps
----------------

#. make_temporary_directories
#. prepare_image
#. load_gadget_yaml
#. populate_rootfs_contents
#. populate_rootfs_contents_hooks
#. generate_disk_info
#. calculate_rootfs_size
#. populate_bootfs_contents
#. populate_prepare_partitions
#. make_disk
#. generate_manifest
#. finish

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
.. _`gadget tree`: Example: https://github.com/snapcore/pc-gadget
.. _`gadget.yaml`: https://github.com/snapcore/snapd/wiki/Gadget-snap#gadget.yaml
.. _`image_definition.yaml`: https://github.com/canonical/ubuntu-image/tree/main/internal/imagedefinition#readme
