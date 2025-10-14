# Security policy

## Ubuntu-image and images

Ubuntu-image is a tool that generates system images. Images are typically composed of
Ubuntu software and application code, and without regular maintenance and updates they
can become vulnerable.

An image's author or maintainer is the sole party responsible for its security. Image
authors should be diligent and keep the software inside their images up-to-date with the
latest releases, security patches, and security measures.

Any vulnerabilities found in an image should be reported to the image's author or
maintainer.

## Supported Versions

We support the last version published in the `latest/stable` channel of the [Snapstore](https://snapcraft.io/ubuntu-image).

Previous major versions (1.X and 2.X) are not supported. 

## Reporting a Vulnerability

To report a security issue, file a [Private Security Report] with a description of the
issue, the steps you took to create the issue, affected versions, and, if known,
mitigations for the issue.

The [Ubuntu Security disclosure and embargo policy] contains more information about
what you can expect when you contact us and what we expect from you.

[Private Security Report]: https://github.com/canonical/imagecraft/security/advisories/new
[Ubuntu Security disclosure and embargo policy]: https://ubuntu.com/security/disclosure-policy