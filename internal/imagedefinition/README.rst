================
Image Definition
================

The image definition is a YAML file that is consumed by ``ubuntu-image``
that specifies how to build a classic image.

The following specification defines what is supported in the YAML:

.. code:: yaml

    # The name of the image.
    name: <string>
    # The human readable name to use in the image.
    display-name: <string>
    # An integer used to track changes to the image definition file.
    revision: <int> (optional)
    # The architecture of the image to create.
    architecture: amd64 | armhf | arm64 | s390x | ppc64el | riscv64
    # The Ubuntu codename to use as apt sources. Example: noble
    series: <string>
    # The classification for this image.
    class: cloud | installer | preinstalled
    # An alternative kernel to install in the image. Normally this
    # is just one kernel and defaults to "linux", but we support
    # installing more than one, since installer images can provide
    # multiple kernels to choose from.
    kernel: <string> (optional)
    # gadget defines the boot assets of an image. When building a
    # classic image, the gadget is optionally compiled as part of
    # the state machine run.
    gadget: (optional)
      # An URI pointing to the location of the gadget tree. For
      # gadgets that need to be built this can be a local path
      # to a directory or a URL to be git cloned. These gadget
      # tree builds will automatically have the environment
      # variables ARCH=<architecture> and SERIES=<series>.
      # The values for these environment variables are sourced
      # from this image definition file. For pre-built
      # gadget trees this must be a local path.
      # The URI must begin with either http://, https://, or file://
      url: <string>
      # The type of gadget tree source. Currently supported values
      # are git, directory, and prebuilt. When git is used the url
      # will be cloned and `make` will be run. When directory is
      # used, ubuntu-image will change directories into the specified
      # URL and run `make`. When prebuilt is used, the contents of the
      # URL are simply copied to the gadget directory.
      type: git | directory | prebuilt
      # A git reference to use if building a gadget tree from git.
      ref: <string> (optional)
      # The branch to use if building a gadget tree from git.
      # Defaults to "main"
      branch: <string> (optional)
      # The target to build when running "make". If none is specified
      # make will be called with no target. This key/value pair has
      # no effect when the gadget.type is "prebuilt"
      target: <string> (optional)
    # A path to a model assertion to use when pre-seeding snaps
    # in the image. Must be a local file URI beginning with file://
    # The given path will be interpreted as relative to the path of
    # the image definition file if is not absolute.
    model-assertion: <string> (optional)
    # Defines parameters needed to build the rootfs for a classic
    # image. Currently only building from a seed is supported.
    # Exactly one of the following must be included: seed,
    # archive-tasks, or tarball.
    rootfs:
      # Components are a list of apt sources, such as main,
      # universe, and restricted. Defaults to "main,restricted".
      components: (optional)
        - <string>
        - <string>
      # The archive to use as an apt source. Defaults to "ubuntu".
      archive: <string> (optional)
      # The flavor of Ubuntu to build. Examples: kubuntu, xubuntu.
      # Defaults to "ubuntu".
      flavor: <string> (optional)
      # The mirror for apt sources.
      # Defaults to "http://archive.ubuntu.com/ubuntu/".
      mirror: <string> (optional)
      # Ubuntu offers several pockets, which often imply the
      # inclusion of other pockets. The release pocket only
      # includes itself. The security pocket includes itself
      # and the release pocket. Updates includes updates,
      # security, and release. Proposed includes all pockets.
      # Defaults to "release".
      pocket: release | security | updates | proposed (optional)
      # Used for building an image from a set of archive tasks
      # rather than seeds. Not yet supported.
      archive-tasks: (exactly 1 of archive-tasks, seed or tarball must be specified)
        - <string>
        - <string>
      # The seed to germinate from to create a list of packages
      # to be installed in the image.
      seed: (exactly 1 of archive-tasks, seed or tarball must be specified)
        # A list of git, bzr, or http locations from which to
        # retrieve the seeds. Must be a web address, local file paths
        # are not supported
        urls: (required if seed dict is specified)
          - <string>
          - <string>
        # The names of seeds to use from the germinate output.
        # Examples: server, minimal, cloud-image.
        names: (required if seed dict is specified)
          - <string>
          - <string>
        # Whether to use the --vcs flag when running germinate.
        # Defaults to "true".
        vcs: <boolean> (optional)
        # An alternative branch to use while retrieving seeds
        # from a git or bzr source.
        branch: <string> (optional)
      # Whether to write the sources list as Deb822 formatted entries in 
      # /etc/apt/sources.list.d/ubuntu.sources or not (and thus use the legacy format
      # in /etc/apt/sources.list)
      # Default to "false" for now to not break existing builds but a warning will be
      # displayed and this default will switch at some point in the future.
      # A warning is also displayed if no value was explicitely set for this field.
      sources-list-deb822: <boolean> (optional)
      # Used for pre-built root filesystems rather than germinating
      # from a seed or using a list of archive-tasks. Must be an
      # an uncompressed tar archive or a tar archive with one of the
      # following compression types: bzip2, gzip, xz, zstd.
      tarball: (exactly 1 of archive-tasks, seed or tarball must be specified)
        # The path to the tarball. Currently only local paths beginning with
        # file:// are supported. The given path will be interpreted as relative
        # to the path of the image definition file if is not absolute.
        url: <string> (required if tarball dict is specified)
        # URL to the gpg signature to verify the tarball against.
        gpg: <string> (optional)
        # SHA256 sum of the tarball used to verify it has not
        # been altered.
        sha256sum: <string> (optional)
    # ubuntu-image supports building automatically with some
    # customizations to the image. Note that if customization
    # is specified, at least one of the subkeys should be used
    # This is only supported for classic image building 
    customization: (optional)
      # Components are a list of apt sources, such as main,
      # universe, and restricted. Defaults to "main, restricted, universe".
      # These are used in the resulting img, not to build it.
      components: (optional)
        - <string>
        - <string>
      # Ubuntu offers several pockets, which often imply the
      # inclusion of other pockets. The release pocket only
      # includes itself. The security pocket includes itself
      # and the release pocket. Updates includes updates,
      # security, and release. Proposed includes all pockets.
      # Defaults to "release".
      # This value is in the resulting img, not to build it.
      pocket: release | security | updates | proposed (optional)
      # Used only for installer images
      installer: (optional)
        preseeds: (optional)
          - <string>
          - <string>
        # Only applicable to subiquity based layered images.
        layers: (optional)
          - <string>
          - <string>
      # Used to create a custom cloud-init configuration.
      # Given configuration should be fully valid cloud-init configuration
      # (including file header) 
      cloud-init: (optional)
        # cloud-init yaml metadata
        meta-data: <yaml as a string> (optional)
        # cloud-init yaml metadata
        user-data: <yaml as a string> (optional)
        # cloud-init yaml metadata
        network-config: <yaml as a string> (optional)
      # Extra PPAs to install in the image. Both public and
      # private PPAs are supported. If specifying a private
      # PPA, the auth and fingerprint fields are required.
      # For public PPAs, auth has no effect and fingerprint
      # is optional. These PPAs will be used as a source
      # while creating the rootfs for the classic image.
      extra-ppas: (optional)
        -
          # The name of the PPA in the format "user/ppa-name".
          name: <string>
          # The fingerprint of the GPG signing key for this
          # PPA. Public PPAs have this information available
          # from the Launchpad API, so it can be retrieved
          # automatically. For Private PPAs this must be
          # specified.
          fingerprint: <string> (optional for public PPAs)
          # Authentication for private PPAs in the format
          # "user:password".
          auth: <string> (optional for public PPAs)
          # Whether to leave the PPA source file in the resulting
          # image. Defaults to "true". If set to "false" this
          # PPA will only be used as a source for installing
          # packages during the rootfs build process, and the
          # resulting image will not have this PPA configured.
          keep-enabled: <boolean>
      # A list of extra packages to install in the rootfs beyond
      # what is included in the germinate output.
      extra-packages: (optional)
        -
          name: <string>
      # Extra snaps to preseed in the rootfs of the image.
      extra-snaps: (optional)
        -
          # The name of the snap.
          name: <string>
          # The channel from which to seed the snap.
          # If both the revision and channel are provided
          # the snap revision specified will be installed
          # and updates will come from the channel specified
          channel: <string> (optional)
          # The store to retrieve the snap from. Not yet supported.
          # Defaults to "canonical".
          store: <string> (optional)
          # The revision of the snap to preseed in the rootfs.
          # If both the revision and channel are provided
          # the snap revision specified will be installed
          # and updates will come from the channel specified
          revision: <int> (optional)
      # After the rootfs has been created and before the image
      # artifacts are generated, ubuntu-image can automatically
      # perform some manual customization to the rootfs.
      manual: (optional)
        # Create directories in the rootfs of the image
        make-dirs: (optional)
          -
            # The path to the directory to create
            # Every intermediate directories missing on the path
            # will be created.
            path: <string>
            # Permissions to give to the directory and any missing
            # intermediate directories.
            permissions: <uint32>
        # Copies files from the host system to the rootfs of
        # the image.
        copy-file: (optional)
          -
            # The path to the file to copy.
            # The given path will be interpreted as relative to the
            # path of the image definition file if is not absolute.
            source: <string>
            # The path to use as a destination for the copied
            # file. The location of the rootfs will be prepended
            # to this path automatically.
            destination: <string>
        # Creates empty files in the rootfs of the image.
        touch-file: (optional)
          -
            # The location of the rootfs will be prepended to this
            # path automatically.
            path: <string>
        # Chroots into the rootfs and executes an executable file.
        # This customization state is run after the copy-files state,
        # so files that have been copied into the rootfs are valid
        # targets to be executed.
        execute: (optional)
          -
            # Path inside the rootfs.
            path: <string>
        # Any additional users to add in the rootfs
        # We recommend using cloud-init when possible and fallback
        # on this method if not possible (e.g performance issues)
        add-user: (optional)
          -
            # The name for the user
            name: <string>
            # The UID to assing to this new user
            id: <string> (optional)
            # Password. This can be a plain text or a hashed value.
            # This password will immediately expire and force the user to 
            # renew it at first login.
            password: <string> (optional)
            # Type of password submitted above. Defaults to "hash" 
            password-type: text | hash (optional)
        add-group: (optional)
          -
            # The name of the group to create.
            name: <string>
            # The GID to assign to this group.
            gid: <string> (optional)
      # Set a custom fstab. The existing one (if any) will be truncated.
      fstab: (optional)
        -
          # the value of LABEL= for the fstab entry
          label: <string>
          # where to mount the partition
          mountpoint: <string>
          # the filesystem type
          filesystem-type: <string>
          # options for mounting the filesystem
          mount-options: <string> (optional)
          # whether or not to dump the filesystem
          dump: <bool> (optional)
          # the order to fsck the filesystem
          fsck-order: <int>
    # Define the types of artifacts to create, including the actual images,
    # manifest files, changelogs, and a list of files in the rootfs.
    # If this is not set, only the rootfs will be created.
    artifacts: (optional)
      # Used to specify that ubuntu-image should create a .img file.
      img: (optional)
        -
          # Name to output the .img file.
          name: <string>
          # Volume from the gadget from which to create the image
          volume: <string> (optional for single volume gadgets,
                            required for multi-volume gadgets)
      # Used to specify that ubuntu-image should create a .iso file.
      # Not yet supported.
      iso: (optional)
        -
          # Name to output the .iso file.
          name: <string>
          # Volume from the gadget from which to create the image
          volume: <string> (optional for single volume gadgets,
                            required for multi-volume gadgets)
          # Specify parameters to use when calling `xorriso`. When not
          # provided, ubuntu-image will attempt to create it's own
          # `xorriso` command.
          xorriso-command: <string> (optional)
      # Used to specify that ubuntu-image should create a .qcow2 file.
      # If a .img file is specified for the corresponding volume, the
      # existing .img will be re-used and converted into a qcow2 image.
      # Otherwise, a new raw image will be created and then converted
      # to qcow2.
      qcow2: (optional)
        -
          # Name to output the .qcow2 file.
          name: <string>
          # Volume from the gadget from which to create the image
          volume: <string> (optional for single volume gadgets,
                            required for multi-volume gadgets)
      # A manifest file is a list of all packages and their version
      # numbers that are included in the rootfs of the image.
      manifest:
        # Name to output the manifest file.
        name: <string>
      # A filelist is a list of all files in the rootfs of the image.
      filelist:
        # Name to output the filelist file.
        name: <string>
      # Not yet supported.
      changelog:
        name: <string>
      # A tarball of the rootfs that has been built by ubuntu-image.
      rootfs-tarball:
        # Name to output the tar archive.
        name: <string>
        # Type of compression to use on the tar archive. Defaults
        # to "uncompressed"
        compression: uncompressed (default) | bzip2 | gzip | xz | zstd (optional)

The following sections detail the top-level keys within this definition,
followed by several examples.


name
====

This mandatory meta-data field is not yet used, but must not be blank.
Any characters are permitted, of any (non-zero) length. For example:

.. code:: yaml

    name: ubuntu-server-raspi


display-name
============

This mandatory meta-data field is not yet used, but must not be blank.
Any characters are permitted, of any (non-zero) length. For example:

.. code:: yaml

    display-name: Ubuntu Server for Raspberry Pi


revision
========

This optional meta-data field is not yet used. If specified, it must
be an integer number.


architecture
============

This mandatory field specifies the architecture of the image to be created. It
must be one of the following valid strings:

* amd64
* armhf
* arm64
* s390x
* ppc64el
* riscv64

For example:

.. code:: yaml

    architecture: arm64


series
======

This mandatory field specifies the Ubuntu release name as it should appear in
apt sources. For example, to produce an image for the 24.04 release, this
should be "noble". Example values include:

* noble
* oracular
* focal
* jammy

Please consult the `Releases <https://wiki.ubuntu.com/Releases>`_ page for
currently valid release names, but bear in mind that release names must be
specified as they would appear in apt sources, i.e. lower-cased with no numeric
part and no "LTS" suffix.

For example:

.. code:: yaml

    series: noble


class
=====

This mandatory field specifies the image classification. It is currently
unused, and must be set to the string "preinstalled". In future, the set of
valid strings is intended to be:

* preinstalled
* installer
* cloud

For example:

.. code:: yaml

    class: preinstalled


kernel
======

This optional key specifies an additional kernel to include in the image. If
specified, the value should be a string that represents the name of the
kernel package to be installed.

.. code:: yaml

    kernel: linux-image-generic


gadget
======

This optional field specifies from where the gadget tree will be sourced.
Support is included for prebuilt gadgets, building gadgets from a local
directory, or building gadgets from a git repository. If gadget is not
included in the image definition, but some disk output (img, qcow2, iso)
is included, an error will occur. Gadget should only be excluded if the
only artifact that you will be creating is a rootfs tarball.

Examples
========

Note that not all of these fields are required. An example used to build
Raspberry Pi images is:

.. code:: yaml

  name: ubuntu-server-raspi-arm64
  display-name: Ubuntu Server Raspberry Pi arm64
  revision: 2
  architecture: arm64
  series: noble
  class: preinstalled
  kernel: linux-image-raspi
  gadget:
    url: "https://git.launchpad.net/snap-pi"
    branch: "classic"
    type: "git"
  rootfs:
    archive: ubuntu
    sources-list-deb822: true
    components:
      - main
      - restricted
      - universe
      - multiverse
    mirror: "http://ports.ubuntu.com/ubuntu-ports/"
    pocket: updates
    seed:
      urls:
        - "git://git.launchpad.net/~ubuntu-core-dev/ubuntu-seeds/+git/"
      branch: noble
      names:
        - server
        - server-raspi
        - raspi-common
        - minimal
        - standard
        - cloud-image
        - supported-raspi-common
  customization:
    cloud-init:
      user-data: |
        #cloud-config
        chpasswd:
          expire: true
          users:
            - name: ubuntu
              password: ubuntu
              type: text
    extra-snaps:
      - name: snapd
    fstab:
      - label: "writable"
        mountpoint: "/"
        filesystem-type: "ext4"
        dump: false
        fsck-order: 1
      - label: "system-boot"
        mountpoint: "/boot/firmware"
        filesystem-type: "vfat"
        mount-options: "defaults"
        dump: false
        fsck-order: 1
  artifacts:
    img:
      - name: ubuntu-24.04-preinstalled-server-arm64+raspi.img
    manifest:
      name: ubuntu-24.04-preinstalled-server-arm64+raspi.manifest
