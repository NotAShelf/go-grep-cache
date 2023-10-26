# go-grep-cache

> Go port of https://github.com/delroth/grep-nixos-cache

"A tool to efficiently grep the contents of many NixOS store paths for a given string to find. The main use case is looking for vendored libraries through the entirety of a Hydra evaluation."

**I'm petty, sue me.**

# How to use

```console
grep-nixos-cache --needle "what-to-search" --path ""/nix/store/...""
```

# Why does this exist?

1. I don't want Rust on my system
2. I don't want flake-utils on my system
3. Felt like it
