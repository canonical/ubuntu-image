## Autopkgtest

In order to run the autopkgtest suite localy you need first to generate an image:

    $ autopkgtest-buildvm-ubuntu-cloud -a amd64 -r bionic -v

This will create a `adt-xenial-amd64-cloud.img` file, then you can run the tests from
the project's root with:

    $ autopkgtest . -- qemu ./autopkgtest-bionic-amd64.img
