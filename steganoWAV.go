// Copyright (c) 2012 Stéphane Bunel. All rights reserved.
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
	"os"
)

const (
	MAJOR    = 1
	MINOR    = 0
	REVISION = 0
	APP      = "steganoWAV"
)

type global_data struct {
	data_file   string // Path to data file
	wave_file   string // Path to WAVE/PCM file
	cmd_extract bool   // Is extract action ?
	cmd_hide    bool   // Is hide action ?
	density     uint   // Bits used per bytes to hide data: 1, 2 or 4
	offset      uint   // In sample. This is your SECRET key
}

type wave_info_struct struct {
	file_type_bloc_id string // ='RIFF'
	file_size         uint32 // Total file size = file_size + 8
	file_format_id    string // = 'WAVE
	format_bloc_id    string // = 'fmt '
	bloc_size         uint32 // = 16
	audio_format      uint16 // = 1 for PCM not compressed
	nbr_cannaux       uint16 //
	frequence         uint32 //
	byte_per_sec      uint32 //
	byte_per_bloc     uint16 //
	bits_per_sample   uint16 //
	data_bloc_id      string //
	data_bloc_size    uint32 //
	bytes_per_sample  int    // = bits_per_sample >> 3
}

type wave_handler_struct struct {
	filename     string           // wave_file
	file         *os.File         // os.File(wave_file)
	dataf        *os.File         // os.File(data_file)
	wave_info    wave_info_struct // wave_info_struct
	first_sample uint32           // 44
}

var (
	VERSION string = fmt.Sprintf("V%d.%d.%d", MAJOR, MINOR, REVISION)
	gd             = &global_data{}
)

//-----------------------------------------------------------------------
// METHODS on *global_data_structure
//-----------------------------------------------------------------------

// 
func (self *global_data) init() {
	var (
		print_usage bool = false
	)

	if self.wave_file == "" {
		fmt.Fprintln(os.Stderr, "-wav is mandatory")
		print_usage = true
	}

	if (self.cmd_extract == true && self.cmd_hide == true) ||
		(self.cmd_extract == false && self.cmd_hide == false) {
		fmt.Fprintf(os.Stderr, "Need exclusive action: -hide OR -extract\n")
		print_usage = true
	}

	if self.cmd_hide && self.data_file == "" {
		fmt.Fprintln(os.Stderr, "-data is mandatory with -hide action")
		print_usage = true
	}

	if print_usage {
		show_usage()
		os.Exit(1)
	}
}

// Show usage of APP
func show_usage() {
	fmt.Fprintf(os.Stderr, "%s %s\n\n", APP, VERSION)
	fmt.Fprintf(os.Stderr,
		"Usage of %s:\t -wav=<filename> [-extract] [-hide] "+
			"[-data=<filename>] [-density=<1|2|4>] [-offset=<uint>]\n\n",
		os.Args[0])
	flag.PrintDefaults()
}

//-----------------------------------------------------------------------
//-- METHODS on *wave_handler_struct
//-----------------------------------------------------------------------

// Open and decode/check WAVE Audio file to work with.
func (self *wave_handler_struct) Open(filename string, write bool) (err error) {
	var (
		flags int = os.O_RDONLY
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

// Hide put given data into a WAVE Audio file
func (self *wave_handler_struct) Hide(offset uint32, input *os.File) (err error) {
	// Offset is expressed as sample
	offset *= uint32(self.wave_info.bytes_per_sample)
	pos := self.first_sample + offset
	b := byte(0)

	newpos, err := self.file.Seek(int64(pos), os.SEEK_SET)
	if err != nil {
		return err
	}
	pos = uint32(newpos)

	// Is there enough space ?
	size_left := self.wave_info.data_bloc_size - uint32(offset)
	samples_left := size_left / uint32(self.wave_info.bytes_per_sample)
	hidden_bytes_left := samples_left / uint32(8/gd.density)

	// Get file size
	fi, err := input.Stat()
	if err != nil {
		return err
	}
	data_size := fi.Size()
	if data_size >= (1 << 33) {
		return errors.New("WAVE/PCM cannot handle file bigger tha 4Gibi!")
	}

	if uint32(data_size) > hidden_bytes_left {
		return errors.New(fmt.Sprintf("Size of data to hide (%d) is bigger than maximum (%d).",
			data_size, hidden_bytes_left))
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

// hide_byte Byte is splitted as binary fields and written in necessary samples LSB.
func (self *wave_handler_struct) hide_byte(b byte) (err error) {
	var (
		shift                    uint   = gd.density
		mask                     byte   = (1 << gd.density) - 1
		samples_to_hide_one_byte uint   = (8 / gd.density)
		stmp                     []byte = []byte{0}
		s                        byte   = 0
		skip                     int64  = int64(self.wave_info.bytes_per_sample) - 1
	)

	for i := uint(0); i < samples_to_hide_one_byte; i++ {
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
	size_left := self.wave_info.data_bloc_size - uint32(offset)
	samples_left := size_left / uint32(self.wave_info.bytes_per_sample)
	hidden_bytes_left := samples_left / uint32(8/gd.density)
	if data_size > hidden_bytes_left {
		return errors.New(fmt.Sprintf("Consistency error. "+
			"Size of data to extract (%d) is bigger than maximum (%d). Maybe a wrong offset ?",
			data_size, hidden_bytes_left))
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
		mask                        = byte(1<<gd.density) - 1
		shift                       = uint(gd.density)
		samples_to_extract_one_byte = uint(8 / gd.density)
		s                           = byte(0)
		skip                        = int64(self.wave_info.bytes_per_sample) - 1
	)

	// Loop over samples
	for i := uint(0); i < samples_to_extract_one_byte; i++ {
		// Sample is little endian ordered. Only first byte of them is usefull
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
	)

	if err = binary.Read(self.file, binary.LittleEndian, &quad); err != nil {
		return err
	}
	self.wave_info.file_type_bloc_id = string(quad[:4])
	//log.Printf("file_type_bloc_id = \"%s\"\n", info.file_type_bloc_id)

	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.file_size); err != nil {
		return err
	}
	//log.Printf("file_size = %d + 8\n", info.file_size)

	if err = binary.Read(self.file, binary.LittleEndian, &quad); err != nil {
		return err
	}
	self.wave_info.file_format_id = string(quad[:4])
	//log.Printf("file_format_id = \"%s\"\n", info.file_format_id)

	// The "WAVE" format consists of two subchunks: "fmt " and "data":
	// The "fmt " subchunk describes the sound data's format:
	if err = binary.Read(self.file, binary.LittleEndian, &quad); err != nil {
		return err
	}
	self.wave_info.format_bloc_id = string(quad[:4])
	//log.Printf("\nformat_bloc_id = \"%s\"\n", info.format_bloc_id)

	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.bloc_size); err != nil {
		return err
	}
	//log.Printf("bloc_size = %d\n", info.bloc_size)

	// 1 = PCM not compressed    
	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.audio_format); err != nil {
		return err
	}
	//log.Printf("audio_format = %d%s\n", info.audio_format, msg)

	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.nbr_cannaux); err != nil {
		return err
	}
	//log.Printf("nbr_cannaux = %d\n", info.nbr_cannaux)

	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.frequence); err != nil {
		return err
	}
	//log.Printf("frequence = %d\n", info.frequence)

	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.byte_per_sec); err != nil {
		return err
	}
	//log.Printf("byte_per_sec = %d\n", info.byte_per_sec)

	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.byte_per_bloc); err != nil {
		return err
	}
	//log.Printf("byte_per_bloc = %d\n", info.byte_per_bloc)

	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.bits_per_sample); err != nil {
		return err
	}
	//log.Printf("bits_per_sample = %d\n", info.bits_per_sample)

	if err = binary.Read(self.file, binary.LittleEndian, &quad); err != nil {
		return err
	}
	self.wave_info.data_bloc_id = string(quad[:4])
	//log.Printf("\ndata_bloc_id = \"%s\"\n", info.data_bloc_id)

	if err = binary.Read(self.file, binary.LittleEndian, &self.wave_info.data_bloc_size); err != nil {
		return err
	}
	//log.Printf("data_bloc_Size = %d\n", info.data_bloc_size)

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

	self.wave_info.bytes_per_sample = int(self.wave_info.bits_per_sample >> 3)

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
	var (
		wh    = &wave_handler_struct{}
		dataf *os.File
		err   error
	)

	// Parse command line arguments 
	flag.BoolVar(&gd.cmd_extract, "extract", false, "Extract hidden data from WAV Audio file to stdout")
	flag.BoolVar(&gd.cmd_hide, "hide", false, "Hide data into WAV Audio file")
	flag.StringVar(&gd.wave_file, "wave", "", "WAVE Audio file to work with")
	flag.StringVar(&gd.data_file, "data", "", "File containing data to hide")
	flag.UintVar(&gd.density, "density", 2, "Bits used per sample: 1, 2 or 4")
	flag.UintVar(&gd.offset, "offset", 0, "Sample offset. *Offset is your secret key!*")
	flag.Usage = show_usage
	flag.Parse()

	// Init global data structure
	gd.init()

	// Open WAVE Audio file
	if err = wh.Open(gd.wave_file, gd.cmd_hide); err != nil {
		log.Fatal(fmt.Sprintf("Cannot open \"%s\": %s\n", gd.wave_file, err))
	}

	// Execute action
	switch {
	case gd.cmd_extract:
		if err = wh.Extract(uint32(gd.offset), os.Stdout); err != nil {
			log.Fatal(fmt.Sprintf("%s\n", err))
		}
	case gd.cmd_hide:
		// Open DATA file
		if dataf, err = os.OpenFile(gd.data_file, os.O_RDONLY, 0); err == nil {
			if err = wh.Hide(uint32(gd.offset), dataf); err != nil {
				log.Fatal(fmt.Sprintf("%s\n", err))
			}
		} else {
			log.Fatal(fmt.Sprintf("Cannot open \"%s\": \"%s\"\n", gd.data_file, err))
		}
	}
	wh.free()
}
