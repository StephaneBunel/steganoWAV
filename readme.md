steganoWAV
==========

steganoWAV is a tool for hide/extract your data into/from a WAVE/PCM audio file.

building
========

go build -ldflags "-s" steganoWAV.go

Usage
=====
Usage                  : ./steganoWAV <ACTION> [<OPTIONS>]

ACTIONS:
  --help               : Show this command summary.
  --version             : Show version informations.
  --info                : Print informations about given WAVE Audio file (need --wave option).
  --extract             : Extract data from given WAVE Audio file to stdout (need --wave, --offset options).
  --hide                : Hide data into given WAVE Audio file (need --payload, --wave, --offset options).

OPTIONS:
  --wave=<filename>    : Path to WAVE/PCM Audio file.
  --payload=<filename>  : Path to file containing data to hide.
  --density=<integer>   : Must be 1, 2, 4 or 8 (default to AUTO).
  --offset=<integer>    : Must be > 0. This is one of your SECRETS.
  --obfuscate=<integer> : Use a Fibonacci generator to obfuscate payload. This is one of your SECRETS.

Examples:
  Get informations about capsule:
  $ steganoWAV --wave=boris24.2.wav --offset=5432 --info

  Hide source code of steganoWAV:
  $ steganoWAV --wave=boris24.2.wav --payload=steganoWAV.go --offset=5432 --obfuscate=10 --hide

  Extract source code to stdout:
  $ steganoWAV --wave=boris24.2.wav --offset=5432 --obfuscate=10 --extract

Examples (Linux)
----------------

  $ echo "My very secret data" >secret.txt

Hide your secret inside a WAVE Audio file.
Without changing what you hear when listening the file. Of course!

  $ steganoWAV --wave=boris.wav --payload=secret.txt --offset=5432 --obfuscate=10 --hide 

Move your sensible file in a secure location:

  $ rm secret.txt

When you need, extract your secrets from your WAVE Audio file:

  $ steganoWAV --wave=boris.wav --offset=5432 --obfuscate=10 --extract
  My very secret data

