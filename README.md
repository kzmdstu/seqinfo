# seqinfo

seqinfo gathers info from sequences in directories, and prints or writes it to an excel file.

## Usage

```
seqinfo /path/to/search/sequences
```

For advanced uses, see the help.

```
seqinfo -help
```

## oiiotool

It uses `oiiotool` as you can see in config.toml, which isn't neccesary but recommended.

You can install `oiiotool` from epel-release in RockyLinux.

```
dnf install epel-release
dnf install OpenImageIO-utils
```

You can also install it in a Mac using Homebrew.

```
brew install openimageio
```
