# How to run this

Basically, run the following: `goss --vars-inline "Ip: $EXTERNAL_IP" v`

## unknown-top-level-key.yaml

A small, self-contained spec demonstrating the unknown-top-level-key
warning (see [docs/gossfile.md](../docs/gossfile.md#unknown-top-level-keys)).
Run it directly, no vars needed:

```console
syver -g examples/unknown-top-level-key.yaml validate
```

It shows both sides: a typo'd key that produces a `[WARN]`, and a
legitimate top-level YAML anchor next to it that does not.
