FROM alpine:3.24

# Upgrade every package at build time. The upstream base image is republished
# infrequently, and its point-release tags can share a digest with the minor tag
# for months, so a rebuild on its own inherits whatever package set was baked in
# back then, advisories included. Upgrading wholesale rather than pinning the
# fixed version of an individual package keeps this line correct as the package
# set moves; a pinned version goes stale as fast as the advisory that prompted
# it. docs/container_image.md offers this image as a base image, so anything
# left unpatched here is inherited by every downstream FROM.
RUN apk --no-cache upgrade

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/syver /usr/bin/

RUN mkdir /syver /goss
VOLUME /syver
VOLUME /goss
