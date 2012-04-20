// Copyright (C) 2012 Stéphane Bunel. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
//    * Redistributions of source code must retain the above copyright
//      notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
//      copyright notice, this list of conditions and the following disclaimer
//      in the documentation and/or other materials provided with the
//      distribution.
//    * Neither the name of author nor the names of its contributors
//      may be used to endorse or promote products derived from this
//      software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
//
//-- 2012-04-08, Stéphane Bunel < stephane [@] bunel [.] org >
//--           * I wrote steganoWAV as an exercise to learn the GO programming
//--             language. It's my first try ;)
//-- 2012-04-20, Stéphane Bunel < stephane [@] bunel [.] org >
//--           * Add new actions: --info and --version
//--           * TODO: Use memory buffers to drastically speed up hide/extract.
//
// Building:
// go build -ldflags "-s" steganoWAV.go
//

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"
)

const (
	MAJOR    = 1
	MINOR    = 1
	REVISION = 0
	APP      = "steganoWAV"
)

const (
	ACTION_HELP = iota
	ACTION_VERSION
	ACTION_INFO
	ACTION_EXTRACT
	ACTION_HIDE
)

type global_data struct {
	action    uint   // Action to run
	data_file string // Path to data file
	wave_file string // Path to WAVE/PCM file
	density   uint32 // Bits used per bytes to hide data: 1, 2, 4 or 8
	offset    uint32 // In sample. This is one of your SECRET
}

type wave_info_struct struct {
	file_type_bloc_id string        // ='RIFF'
	file_size         uint32        // Total file size = file_size + 8
	file_format_id    string        // = 'WAVE
	format_bloc_id    string        // = 'fmt '
	fmt_bloc_size     uint32        // = 16
	audio_format      uint32        // = 1 for PCM not compressed
	nbr_cannaux       uint32        //
	frequence         uint32        //
	bytes_per_sec     uint32        //
	byte_per_bloc     uint32        //
	bits_per_sample   uint32        //
	data_bloc_id      string        //
	data_bloc_size    uint32        //
	bytes_per_sample  uint32        // = bits_per_sample >> 3
	total_samples     uint32        // Total number of samples
	sound_duration    time.Duration //
}

type wave_handler_struct struct {
	filename                    string           // wave_file
	file                        *os.File         // os.File(wave_file)
	dataf                       *os.File         // os.File(data_file)
	wave_info                   wave_info_struct // wave_info_struct
	first_sample                uint32           // 44
	samples_for_one_byte        uint32           // # of samples needed to hide a byte
	max_samples_offset          uint32           // Maximum offset in sample
	user_offset_to_first_sample uint32           // = gd.offset
	user_offset_to_first_byte   uint32           // = gd.offset / bytes_per_sample
	user_samples_space          uint32           // # of usable samples
	payload_size                uint32           // # of byte that could be hidden in WAVE Audio file
	user_data_size              uint32           // # of bytes hidden
}

var (
	VERSION   = fmt.Sprintf("%d.%d.%d", MAJOR, MINOR, REVISION)
	EngSuffix = []string{"B", "KiB", "MiB", "GiB"}
	gd        = &global_data{}
)

//-----------------------------------------------------------------------
//-- METHODS on *wave_handler_struct
//-----------------------------------------------------------------------

// Open and decode/check WAVE Audio file to work with.
func (self *wave_handler_struct) Open(filename string, write bool) (err error) {
	var (
		flags = os.O_RDONLY
		f     *os.File
	)

	if write {
		flags = os.O_RDWR
	}

	// Open WAV file
	f, err = os.OpenFile(filename, flags, 0)
	if err != nil {
		return err
	}
	self.file = f
	self.filename = filename

	if err = self.decodeHeaders(); err != nil {
		self.file.Close()
		return err
	}

	return nil
}

// Print some informations about WAV Audio File
func (self *wave_handler_struct) PrintWAVInfo(output *os.File) (err error) {
	msg := ""

	sample_dynamic_at_x_percent := 0.1 * math.Pow(2, float64(self.wave_info.bits_per_sample))
	hiding_dynamic := math.Pow(2, float64(gd.density))
	max_dist := 100.0 * hiding_dynamic / sample_dynamic_at_x_percent

	hidden_start_time := time.Duration(float64(self.user_offset_to_first_byte)/float64(self.wave_info.bytes_per_sec)) * time.Second

	msg = fmt.Sprintf("WAVE Audio file informations\n")
	msg += fmt.Sprintf("============================\n")
	msg += fmt.Sprintf("  File path                      : \"%s\"\n", self.filename)
	msg += fmt.Sprintf("  File size                      : %s (%d bytes)\n", intToSuffixedStr(self.wave_info.file_size+8), self.wave_info.file_size+8)
	msg += fmt.Sprintf("  File type                      : %s\n", self.wave_info.file_type_bloc_id)
	msg += fmt.Sprintf("  RIFF format                    : %s\n", self.wave_info.file_format_id)
	msg += fmt.Sprintf("  Sound data format              : %d\n", self.wave_info.audio_format)
	msg += fmt.Sprintf("  Nbr. of channels               : %d\n", self.wave_info.nbr_cannaux)
	msg += fmt.Sprintf("  Bytes per second               : %s (%d bytes)\n", intToSuffixedStr(self.wave_info.bytes_per_sec), self.wave_info.bytes_per_sec)
	msg += fmt.Sprintf("  Sample freqency                : %d Hz\n", self.wave_info.frequence)
	msg += fmt.Sprintf("  Sample size                    : %d bits (%d bytes)\n", self.wave_info.bits_per_sample, self.wave_info.bytes_per_sample)
	msg += fmt.Sprintf("  Total samples                  : %d\n", self.wave_info.total_samples)
	msg += fmt.Sprintf("  Sound size                     : %s (%d bytes)\n", intToSuffixedStr(self.wave_info.data_bloc_size), self.wave_info.data_bloc_size)
	msg += fmt.Sprintf("  Sound duration                 : %v\n", self.wave_info.sound_duration)

	msg += fmt.Sprintf("\nHiding informations\n")
	msg += fmt.Sprintf("===================\n")
	msg += fmt.Sprintf("  Density                        : %d bits per sample\n", gd.density)
	msg += fmt.Sprintf("    Samples for hide one byte    : %d\n", self.samples_for_one_byte)
	msg += fmt.Sprintf("    Sample alteration @10%% dyn.  : %.5f%%\n", max_dist)
	msg += fmt.Sprintf("  Max sample offset              : %d\n", self.max_samples_offset)
	msg += fmt.Sprintf("    User offset                  : %d (%v)\n", self.user_offset_to_first_sample, hidden_start_time)
	msg += fmt.Sprintf("    Max data size                : %s (%d bytes)\n", intToSuffixedStr(self.payload_size), self.payload_size)

	fmt.Fprintln(output, msg)
	return nil
}

// Hide write input data into a WAVE Audio file
func (self *wave_handler_struct) Hide(offset uint32, input *os.File) (err error) {
	// Offset is expressed as sample
	offset *= self.wave_info.bytes_per_sample
	pos := self.first_sample + offset
	b := byte(0)

	newpos, err := self.file.Seek(int64(pos), os.SEEK_SET)
	if err != nil {
		return err
	}
	pos = uint32(newpos)

	// Get file size
	fi, err := input.Stat()
	if err != nil {
		return err
	}
	data_size := fi.Size()
	self.user_data_size = uint32(data_size)

	if data_size >= (1 << 33) {
		return errors.New("WAVE/PCM format cannot handle file bigger tha 4GiB!")
	}

	// Is there enough space ?
	if uint32(data_size) > self.payload_size {
		return errors.New(fmt.Sprintf("Size of data to hide (%s) is bigger than maximum (%s).",
			intToSuffixedStr(uint32(data_size)), intToSuffixedStr(self.payload_size)))
	}

	// Write data size in first 4 octets
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.LittleEndian, uint32(data_size))
	if err != nil {
		return err
	}
	for _, v := range buf.Bytes() {
		if err = self.hide_byte(v); err != nil {
			return err
		}
	}

	for i := 0; i < int(data_size); i++ {
		if err = binary.Read(input, binary.LittleEndian, &b); err != nil {
			return err
		}
		if err = self.hide_byte(b); err != nil {
			return err
		}
	}

	return nil
}

// hide_byte split byte in binary fields and write it in samples LSB.
func (self *wave_handler_struct) hide_byte(b byte) (err error) {
	var (
		shift        = gd.density
		mask  byte   = (1 << gd.density) - 1
		stmp  []byte = []byte{0}
		s     byte   = 0
		skip  int64  = int64(self.wave_info.bytes_per_sample) - 1
	)

	for i := uint32(0); i < self.samples_for_one_byte; i++ {
		if err = binary.Read(self.file, binary.LittleEndian, &stmp); err != nil {
			return err
		}
		self.file.Seek(-1, os.SEEK_CUR) // Rewind

		s = stmp[0]
		s &= ^mask
		s |= b >> (8 - shift)
		b <<= shift

		// Write byte
		if _, err = self.file.Write([]byte{s}); err != nil {
			return err
		}

		// Jump to next sample
		if _, err = self.file.Seek(skip, os.SEEK_CUR); err != nil {
			return err
		}
	}

	return nil
}

// Extract read hidden data from samples.
func (self *wave_handler_struct) Extract(offset uint32, output *os.File) (err error) {
	var (
		data_size uint32
		b         byte
		tmp       = []byte{0, 0, 0, 0}
		buf       = bytes.NewBuffer(tmp)
	)

	// Jump to beginning of hidden data.
	offset *= uint32(self.wave_info.bytes_per_sample)
	if _, err = self.file.Seek(int64(self.first_sample+offset), os.SEEK_SET); err != nil {
		return err
	}

	// Get size of hidden data to extract (in bytes)
	for i, _ := range tmp {
		if tmp[i], err = self.extract_byte(); err != nil {
			return err
		}
	}
	if err = binary.Read(buf, binary.LittleEndian, &data_size); err != nil {
		return err
	}

	// Check Consistency of data_size
	if data_size > self.payload_size {
		return errors.New(fmt.Sprintf("Consistency error. "+
			"Size of data to extract (%s) is bigger than maximum (%s) payload. Maybe a wrong offset ?",
			intToSuffixedStr(data_size), intToSuffixedStr(self.payload_size)))
	}

	// Extract all data_size bytes
	for data_size != 0 {
		if b, err = self.extract_byte(); err != nil {
			return err
		}
		if _, err = output.Write([]byte{b}); err != nil {
			return err
		}
		data_size--
	}

	return nil
}

// extract_byte read binary fields from samples to recompose a complete byte.
func (self *wave_handler_struct) extract_byte() (b byte, err error) {
	var (
		mask  = byte(1<<gd.density) - 1
		shift = uint(gd.density)
		s     = byte(0)
		skip  = int64(self.wave_info.bytes_per_sample) - 1
	)

	// Loop over samples
	for i := uint32(0); i < self.samples_for_one_byte; i++ {
		// Sample is little endian ordered. Only first byte of them is useful
		if err = binary.Read(self.file, binary.LittleEndian, &s); err != nil {
			return 0, err
		}

		// skip to next sample
		self.file.Seek(skip, os.SEEK_CUR)

		// Make space for new bits
		b <<= shift
		// Filter sample LSBs and add it to recompose a complete byte. 
		b |= s & mask
	}

	return b, nil
}

// decodeHeaders Read the file headers assuming a canonical WAVE format. 
func (self *wave_handler_struct) decodeHeaders() (err error) {
	/*
	 * http://www.lightlink.com/tjweber/StripWav/WAVE.html#WAVE
	 *
	 * The *canonical* WAVE format starts with the RIFF header:
	 * http://ccrma.stanford.edu/courses/422/projects/WaveFormat/
	 */

	var (
		quad = []byte{0, 0, 0, 0}
		pos  = int64(0)
		v16  uint16
		v32  uint32
	)

	// RIFF chunk
	if err = binary.Read(self.file, binary.LittleEndian, &quad); err != nil {
		return err
	}
	self.wave_info.file_type_bloc_id = string(quad[:4])

	// <RIFF bloc size>
	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.file_size); err != nil {
		return err
	}

	// WAVE chunk
	if err = binary.Read(self.file, binary.LittleEndian, &quad); err != nil {
		return err
	}
	self.wave_info.file_format_id = string(quad[:4])

	// The "WAVE" format consists of two subchunks: "fmt " and "data":
	// The "fmt " subchunk describes the sound data's format:
	if err = binary.Read(self.file, binary.LittleEndian, &quad); err != nil {
		return err
	}
	self.wave_info.format_bloc_id = string(quad[:4])

	// <fmt bloc size>
	if err = binary.Read(self.file, binary.LittleEndian, &v32); err != nil {
		return err
	}
	self.wave_info.fmt_bloc_size = v32

	// <audio format> 1 = PCM not compressed    
	if err = binary.Read(self.file, binary.LittleEndian, &v16); err != nil {
		return err
	}
	self.wave_info.audio_format = uint32(v16)

	// <# of channels>
	if err = binary.Read(self.file, binary.LittleEndian, &v16); err != nil {
		return err
	}
	self.wave_info.nbr_cannaux = uint32(v16)

	// <Frequency>
	if err = binary.Read(self.file, binary.LittleEndian, &v32); err != nil {
		return err
	}
	self.wave_info.frequence = v32

	// <Bytes per second>
	if err = binary.Read(self.file, binary.LittleEndian, &v32); err != nil {
		return err
	}
	self.wave_info.bytes_per_sec = v32

	// <byte per bloc>
	if err = binary.Read(self.file, binary.LittleEndian, &v16); err != nil {
		return err
	}
	self.wave_info.byte_per_bloc = uint32(v16)

	// <Bits per sample>
	if err = binary.Read(self.file, binary.LittleEndian, &v16); err != nil {
		return err
	}
	self.wave_info.bits_per_sample = uint32(v16)

	// DATA
	if err = binary.Read(self.file, binary.LittleEndian, &quad); err != nil {
		return err
	}
	self.wave_info.data_bloc_id = string(quad[:4])

	// <data_bloc_size>
	if err = binary.Read(self.file, binary.LittleEndian, &v32); err != nil {
		return err
	}
	self.wave_info.data_bloc_size = v32

	// Store the first sample position (=44 for canonical WAVE Audio file)
	pos, _ = self.file.Seek(0, os.SEEK_CUR)
	self.first_sample = uint32(pos)

	if self.wave_info.file_type_bloc_id != "RIFF" ||
		self.wave_info.file_format_id != "WAVE" ||
		self.wave_info.format_bloc_id != "fmt " ||
		self.wave_info.audio_format != 1 ||
		self.first_sample != 44 {
		return errors.New("Incompatible WAVE file. Must be cannonical RIFF/WAVE/" +
			"PCM (not compressed).")
	}

	if gd.density > self.wave_info.bits_per_sample/2 {
		return errors.New("Density is too high for sample size of this WAVE Audio file.")
	}

	// Compute some values

	self.wave_info.bytes_per_sample = self.wave_info.bits_per_sample >> 3
	//fmt.Printf("self.wave_info.bytes_per_sample = %v\n", self.wave_info.bytes_per_sample)

	self.wave_info.total_samples = self.wave_info.data_bloc_size / self.wave_info.bytes_per_sample
	self.wave_info.sound_duration = time.Duration(float64(self.wave_info.data_bloc_size)/float64(self.wave_info.bytes_per_sec)) * time.Second

	self.user_offset_to_first_sample = gd.offset
	//fmt.Printf("self.user_samples_offset = %v\n", self.user_samples_offset)

	self.user_offset_to_first_byte = gd.offset * self.wave_info.bytes_per_sample
	//fmt.Printf("self.user_bytes_offset = %v\n", self.user_bytes_offset)

	self.samples_for_one_byte = 8 / gd.density
	self.max_samples_offset = self.wave_info.total_samples - (8 * self.samples_for_one_byte)

	payload_samples_space := self.wave_info.total_samples - self.user_offset_to_first_sample
	self.payload_size = payload_samples_space / self.samples_for_one_byte

	// self.max_hiding_capacity = self.wave_info.total_samples / self.samples_for_one_byte
	// self.offseted_hiding_capacity = self.max_hiding_capacity - (self.user_samples_offset / self.samples_for_one_byte)

	if self.user_offset_to_first_sample >= self.max_samples_offset {
		return errors.New("Offset is to big! Retry with bigger WAVE Audio file or reduce offset.")
	}

	return nil
}

// free allocated ressources
func (self *wave_handler_struct) free() {
	if self.file != nil {
		self.file.Close()
	}
	if self.dataf != nil {
		self.dataf.Close()
	}
}

func main() {
	var wh = &wave_handler_struct{}
	var dataf *os.File
	var err error

	// Read cmd line arguments
	parseArgs()
	defer wh.free()

	switch {
	case gd.action == ACTION_HELP:
		show_usage()
		os.Exit(0)
	case gd.action == ACTION_VERSION:
		fmt.Println(APP + " (" + os.Args[0] + ") " + VERSION + ".")
		fmt.Println("Copyright (C) 2012 Stéphane Bunel.")
		fmt.Println("License: BSD style, which is included in the source code.")
		fmt.Println("\n")
		os.Exit(0)
	case gd.action == ACTION_INFO:
		if err = wh.Open(gd.wave_file, false); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open \"%s\": %s\n", gd.wave_file, err)
			break
		}

		if err = wh.PrintWAVInfo(os.Stdout); err != nil {
			log.Fatal(fmt.Sprintf("%s\n", err))
		}
	case gd.action == ACTION_EXTRACT:
		if err = wh.Open(gd.wave_file, false); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open \"%s\": %s\n", gd.wave_file, err)
			break
		}

		if err = wh.Extract(uint32(gd.offset), os.Stdout); err != nil {
			log.Fatal(fmt.Sprintf("%s\n", err))
		}
	case gd.action == ACTION_HIDE:
		if err = wh.Open(gd.wave_file, true); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open \"%s\": %s\n", gd.wave_file, err)
			break
		}

		// Open DATA file
		if dataf, err = os.OpenFile(gd.data_file, os.O_RDONLY, 0); err == nil {
			defer dataf.Close()

			if err = wh.Hide(uint32(gd.offset), dataf); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				break
			}

			first_sample := wh.user_offset_to_first_sample
			last_sample := first_sample + (wh.user_data_size * wh.samples_for_one_byte)
			fmt.Printf("Start %d, end %d\n", first_sample, last_sample)

		} else {
			fmt.Fprintf(os.Stderr, "Failed to open \"%s\": %s\n", gd.data_file, err)
		}
	}
}

func parseArgs() {
	var print_usage = false

	// Parse command line arguments
	flag.Usage = show_usage
	bExtract := flag.Bool("extract", false, "")
	bHide := flag.Bool("hide", false, "")
	bInfo := flag.Bool("info", false, "")
	bVersion := flag.Bool("version", false, "")

	flag.StringVar(&gd.wave_file, "wave", "", "")
	flag.StringVar(&gd.data_file, "data", "", "")
	flag.StringVar(&gd.data_file, "payload", "", "")

	density := flag.Uint64("density", 2, "")
	offset := flag.Uint64("offset", 0, "")

	flag.Parse()

	gd.density = uint32(*density)
	gd.offset = uint32(*offset)

	gd.action = ACTION_HELP
	if *bHide == true {
		gd.action = ACTION_HIDE
	}
	if *bExtract == true {
		gd.action = ACTION_EXTRACT
	}
	if *bInfo == true {
		gd.action = ACTION_INFO
	}
	if *bVersion == true {
		gd.action = ACTION_VERSION
	}

	switch gd.density {
	case 1, 2, 4, 8:
	default:
		fmt.Fprintf(os.Stderr, "Bad value (%v) for -density. Must be 2, 4 or 8. Default: 2\n", gd.density)
		print_usage = true
	}

	if (gd.action == ACTION_INFO || gd.action == ACTION_EXTRACT || gd.action == ACTION_HIDE) && gd.wave_file == "" {
		fmt.Fprintln(os.Stderr, "Option --wave=<filename> is mandatory for this action.")
		print_usage = true
	}

	if (gd.action == ACTION_HIDE || gd.action == ACTION_EXTRACT) && gd.offset == 0 {
		fmt.Fprintln(os.Stderr, "Option --offset=<integer> is mandatory for this action.")
		print_usage = true
	}

	if gd.action == ACTION_HIDE && gd.data_file == "" {
		fmt.Fprintln(os.Stderr, "Option --data=<filename> is mandatory for this action.")
		print_usage = true
	}

	if print_usage {
		show_usage()
		os.Exit(1)
	}
}

// Show usage of APP
func show_usage() {
	fmt.Fprintf(os.Stderr, "\n%s %s\n", APP, VERSION)
	fmt.Fprintf(os.Stderr,
		"Usage                  : %s <ACTION> [<OPTIONS>]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "ACTIONS:")
	fmt.Fprintln(os.Stderr,
		"  --help               : Show this command summary.\n"+
			"  --version            : Show version informations.\n"+
			"  --info               : Print informations about given WAVE Audio file (need --wave option).\n"+
			"  --extract            : Extract data from given WAVE Audio file to stdout (need --wave, --offset options).\n"+
			"  --hide               : Hide data into given WAVE Audio file (need --payload, --wave, --offset options).\n")

	fmt.Fprintln(os.Stderr, "OPTIONS:")
	fmt.Fprintln(os.Stderr,
		"  --wave=<filename>    : Path to WAVE Audio file.\n"+
			"  --payload=<filename> : Path to file containing data to hide.\n"+
			"  --density=<integer>  : Must be 1, 2, 4 or 8 (default to 2).\n"+
			"  --offset=<integer>   : Must be > 0. This is one of your SECRET.\n")

	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  Get informations about capsule:")
	fmt.Fprintln(os.Stderr, "  $ steganoWAV --info --wave=boris24.2.wav --density=4 --offset=5432\n")
	fmt.Fprintln(os.Stderr, "  Hide source code of steganoWAV:")
	fmt.Fprintln(os.Stderr, "  $ steganoWAV --hide --wave=boris24.2.wav --density=4 --offset=5432 --data=steganoWAV.go\n")
	fmt.Fprintln(os.Stderr, "  Extract source code to stdout:")
	fmt.Fprintln(os.Stderr, "  $ steganoWAV --extract --wave=boris24.2.wav --density=4 --offset=5432\n")
}

func intToSuffixedStr(value uint32) (result string) {
	var engorder = 0
	var tempv = float64(value)

	for {
		if value > 1024 {
			engorder += 1
			value >>= 10
			tempv /= 1024
		} else {
			break
		}
	}

	return fmt.Sprintf("%.3f %s", tempv, EngSuffix[engorder])
}
