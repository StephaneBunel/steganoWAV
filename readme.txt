Usage example:
 
$ echo "My very secret data" >secret.txt

# Hide your secret inside a WAVE Audio file.
# Without changing what you hear when listening the file. Of course!
$ ./steganoWAV -wave=boris.wav -offset=5432 -hide -data=secret.txt

# Move your sensible file in a secure location:
$ rm secret.txt

# When you need, extract your secrets from your WAVE Audio file:
$ ./steganoWAV -wave=boris.wav -offset=5432 -extract
My very secret data


# !!! offset IS your SECRET KEY, never forget it, never write it.
# !!! Using a bad offset will produce unpredictable output.
