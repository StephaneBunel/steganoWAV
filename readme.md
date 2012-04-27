steganoWAV
==========

steganoWAV is a tool for hide/extract data into/from a WAVE/PCM audio file.

Most steganographic tools works on images, but audio files are also a good media for that.
Notably the WAVE/PCM files because they are generally far larger than images.
Hence they permit to hide more data per file.

Hiding data in a wave file containing already hidden data will overwrite old data.
As expected, once hidden, stegano WAV has no way to know if a WAVE file already contains hidden data.

Build and install
=================

If necessary, first install Golang (http://code.google.com/p/go/downloads/list) to compile steganoWAV.

    $ go build -ldflags "-s" steganoWAV.go

Once compiled, steganoWAV become a standalone executable file (for your platform) without external
dependency (statically linked). You can rename it and put it anywhere.

Usage
=====

    Usage                   : steganoWAV <ACTION> [<OPTIONS>]
    
    ACTIONS:
      --help                : Show this command summary.
      --version             : Show version informations.
      --info                : Print informations about given WAVE Audio file (need --wave option).
      --extract             : Extract data from given WAVE Audio file to stdout (need --wave, --offset options).
      --hide                : Hide data into given WAVE Audio file (need --payload, --wave, --offset options).
    
    OPTIONS:
      --wave=<filename>     : Path to WAVE/PCM Audio file.
      --payload=<filename>  : Path to file containing data to hide.
      --density=<integer>   : Must be 1, 2, 4 or 8 (default to AUTO).
      --offset=<integer>    : Must be > 0. Must be one of your SECRETS.
      --obfuscate=<integer> : Use a Fibonacci generator to obfuscate payload. Must be one of your SECRETS.
    
    Examples:
      Get informations about capsule:
      $ steganoWAV --wave=boris24.2.wav --payload=steganoWAV.go --offset=5432 --info
    
      Hide source code of steganoWAV:
      $ steganoWAV --wave=boris24.2.wav --payload=steganoWAV.go --offset=5432 --obfuscate=10 --hide
    
      Extract source code to stdout:
      $ steganoWAV --wave=boris24.2.wav --offset=5432 --obfuscate=10 --extract

Examples
========

Linux
-----

    $ echo "My very secret data" >secret.txt

Hide your secret inside a WAVE Audio file.
Without changing what you hear when listening the file. Of course!

    $ steganoWAV --wave=boris.wav --payload=secret.txt --offset=5432 --obfuscate=10 --hide 

Move your sensible file in a secure location:

    $ rm secret.txt

When you need, extract your secrets from your WAVE Audio file:

    $ steganoWAV --wave=boris.wav --offset=5432 --obfuscate=10 --extract
    My very secret data

Mac OS X
--------
 
Hiding:
 
    $ steganoWAV --wave=/Users/toto/Desktop/03RedSister.wav --payload=/Users/toto/Documents/NdF-2012_04.xls --offset=4321 --obfuscate=99 --hide

Extracting:
 
    $ steganoWAV --wave=/Users/toto/Music/03RedSister.wav --offset=4321 --obfuscate=99 --extract >/Users/toto/Desktop/test.xls

Windows
-------

Hiding:

    C:\Users\(...)\steganoWAV>steganoWAV.exe --wave 07Narayan.wav --payload steganoWAV.go --offset 5432 --obfuscate 10 --hide
    Hiding "steganoWAV.go" inside "07Narayan.wav" ...
    Read 27.775 KiB from "steganoWAV.go" and write 222.203 KiB to "07Narayan.wav" in 20.0464ms (10.825 MiB/s).

Extracting:

    C:\Users\(...)\steganoWAV>steganoWAV.exe --wave 07Narayan.wav --offset 5432 --obfuscate 10 --extract
    ...
    (listing of source code)
    ...

Infos about capsule:

    C:\Users\(...)\steganoWAV>steganoWAV.exe --wave 07Narayan.wav --offset 5432 --info
    WAVE Audio file informations
    ============================
      File path                      : "07Narayan.wav"
      File size                      : 45.914 MiB (48144692 bytes)
      Canonical format               : false
      Audio format                   : 1
      Number of channels             : 2
      Sampling rate                  : 44100 Hz
      Bytes per second               : 86.133 KiB (88200 bytes)
      Sample size                    : 8 bits (1 bytes)
      Number of samples              : 48144384
      Sound size                     : 45.914 MiB (48144384 bytes)
      Sound duration                 : 9m5s
    
    Hiding informations
    ===================
      Density                        : 1 bits per sample
        Samples for hide one byte    : 8
        Sample alteration @15% dyn.  : 5.20833% max.
      Max samples offset             : 48111584
        User samples offset          : 5432 (0)
        Max payload size             : 5.739 MiB (6017369 bytes)

Tested platforms
================

SteganoWAV has been reproted to compiles and runs correctly on the following platforms:

  * Linux 2.6 (amd_64, developpement platform).
  * Windows 7 pro (386).
  * Mac OS X 10.6 Snow Leopard (amd_64). Thanks to seblec.
  * Mac OS X 10.7 Lion (amd_64). Thanks to seblec.

FAQ
===

Q: Can I compress a WAVE audio file with hidden data inside ?

A: Yes, but only with a lossless algorithms, like FLAC. By using a lossy algorithm (MP3, OGG, ...) all hidden data will be destroyed.


Q: Can I hide more than one "file" in the same WAVE audio file ?

A: Yes, by adjusting smartly the offset to avoid data overlapping.
Use --info with --wave and --payload to get offset informations.


