# Syver container image

## Dockerfiles

* [latest](https://github.com/krameff/syver/blob/main/Dockerfile)

Release images are published to GitHub Container Registry as
`ghcr.io/<owner>/syver` (for example `ghcr.io/krameff/syver:latest` on tagged
releases, or `ghcr.io/<your-fork-owner>/syver:main` for branch builds).

## Using the base image

This is a simple alpine image with Syver preinstalled on it.
Can be used as a base image for your projects to allow for easy health checking.

### Mount example

Create the container

```sh
docker run --name syver ghcr.io/krameff/syver syver
```

Create your container and mount syver

```sh
docker run --rm -it --volumes-from syver --name weby nginx
```

Run syver inside your container

```sh
docker exec weby /syver/syver autoadd nginx
```

### HEALTHCHECK example

```dockerfile
FROM ghcr.io/krameff/syver:latest

COPY syver/ /syver/
HEALTHCHECK --interval=1s --timeout=6s CMD syver -g /syver/syver.yaml validate

# your stuff..
```

### Startup delay example

```dockerfile
FROM ghcr.io/krameff/syver:latest

COPY syver/ /syver/

# Alternatively, the -r option can be set
# using the SYVER_RETRY_TIMEOUT env variable (GOSS_RETRY_TIMEOUT also works)
CMD syver -g /syver/syver.yaml validate -r 5m && exec real_comand..
```
