FROM alpine:3.24

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/syver /usr/bin/

RUN mkdir /syver /goss
VOLUME /syver
VOLUME /goss
