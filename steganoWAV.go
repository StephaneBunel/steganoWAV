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
//--           * Version 1.0.0
//-- 2012-04-20, Stéphane Bunel < stephane [@] bunel [.] org >
//--           * Add new actions: --info and --version
//--           * TODO: Use memory buffers to drastically speed up hide/extract.
//--           * Version 1.1.0
//-- 2012-04-21, Stéphane Bunel < stephane [@] bunel [.] org >
//--           * Now use memory buffers for drastic speed up.
//-- 2012-04-22, Stéphane Bunel < stephane [@] bunel [.] org >
//--           * Improve RIFF/WAVE parser to skip unknown chunk.
//--           * Version 1.2.1
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
	"io"
	"log"
	"math"
	"os"
	"time"
)

const (
	MAJOR    = 1
	MINOR    = 2
	REVISION = 1
	APP      = "steganoWAV"
)

const (
	ACTION_HELP = iota
	ACTION_VERSION
	ACTION_INFO
	ACTION_EXTRACT
	ACTION_HIDE
)

type PayloadBloc []byte
type SamplesBloc []byte

type global_data struct {
	action    uint   // Action to run
	data_file string // Path to data file
	wave_file string // Path to WAVE/PCM file
	density   uint32 // Bits used per bytes to hide data: 1, 2, 4 or 8
	offset    uint32 // In sample. This is one of your SECRET
}

type wave_info_struct struct {
	audio_format       uint32 // = 1 for PCM not compressed
	num_channels       uint32 //
	sampling_frequency uint32 //
	bytes_per_sec      uint32 //
	byte_per_bloc      uint32 //
	bits_per_sample    uint32 //
	data_bloc_size     uint32 //
	// Computed values
	canonical        bool
	bytes_per_sample uint32        // = bits_per_sample >> 3
	num_samples      uint32        // Total number of samples
	sound_duration   time.Duration //
}

type wave_handler_struct struct {
	wave_info                  wave_info_struct // wave_info_struct
	wave_file_name             string           // Path to WAVE Audio file
	wave_file_size             int64            // Should be < 2^32
	wave_file                  *os.File         // *os.File
	wave_start_offset          uint32           // = gd.offset counted in sample
	wave_start_offset_in_bytes uint32           // = gd.offset * wave_info.bytes_per_sample
	wave_first_sample_pos      uint32           // 44 for canonical RIFF/WAVE
	//
	payload_file_name string   // Path to data file
	payload_file_size int64    //
	payload_file      *os.File // *os.File
	payload_max_size  uint32   // # of byte that could be hidden in WAVE Audio file
	//
	bloc_size            uint32 // Read data by bloc_size step ! Must be set at struct creation
	samples_for_one_byte uint32 // # of samples needed to hide a byte
	wave_max_offset      uint32 // Maximum offset to write one SampleBloc + bloc size
}

var (
	VERSION   = fmt.Sprintf("%d.%d.%d", MAJOR, MINOR, REVISION)
	EngSuffix = []string{"B", "KiB", "MiB", "GiB"}
	gd        = &global_data{}
)

//-----------------------------------------------------------------------
//-- METHODS on *wave_handler_struct
//-----------------------------------------------------------------------

// OpenWave Open the WAVE Audio file and reads header
func (self *wave_handler_struct) OpenWave(filename string, write bool) (err error) {
	var flags = os.O_RDONLY

	if write {
		flags = os.O_RDWR
	}

	// Open WAV file
	self.wave_file_name = filename
	if self.wave_file, err = os.OpenFile(filename, flags, 0); err != nil {
		return err
	}

	// Get system file size
	if fi, err := self.wave_file.Stat(); err == nil {
		self.wave_file_size = fi.Size()
	} else {
		return err
	}

	// Decode WAVE header
	if err = self.parseHeaders(); err != nil {
		return err
	}

	return nil
}

// OpenPayload Open the payload file
func (self *wave_handler_struct) OpenPayload(filename string) (err error) {
	var (
		f         *os.File
		file_size int64
	)

	// Open Payload file
	f, err = os.OpenFile(filename, os.O_RDONLY, 0)
	if err != nil {
		return err
	}

	// Get system file size
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	file_size = fi.Size()

	// Store information
	self.payload_file_name = filename
	self.payload_file_size = file_size
	self.payload_file = f

	return nil
}

// Print some informations about WAV Audio File
func (self *wave_handler_struct) PrintWAVInfo(output *os.File) (err error) {
	msg := ""

	sample_dynamic_at_x_percent := 0.15 * math.Pow(2, float64(self.wave_info.bits_per_sample))
	hiding_dynamic := math.Pow(2, float64(gd.density))
	max_dist := 100.0 * hiding_dynamic / sample_dynamic_at_x_percent

	hidden_start_time := time.Duration(float64(self.wave_start_offset_in_bytes)/float64(self.wave_info.bytes_per_sec)) * time.Second

	msg = fmt.Sprintf("WAVE Audio file informations\n")
	msg += fmt.Sprintf("============================\n")
	msg += fmt.Sprintf("  File path                      : \"%s\"\n", self.wave_file_name)
	msg += fmt.Sprintf("  File size                      : %s (%d bytes)\n", intToSuffixedStr(uint32(self.wave_file_size)), self.wave_file_size)
	//msg += fmt.Sprintf("  Canonical format               : %v\n", self.wave_info.canonical)
	msg += fmt.Sprintf("  Audio format                   : %d\n", self.wave_info.audio_format)
	msg += fmt.Sprintf("  Number of channels             : %d\n", self.wave_info.num_channels)
	msg += fmt.Sprintf("  Sampling rate                  : %d Hz\n", self.wave_info.sampling_frequency)
	msg += fmt.Sprintf("  Bytes per second               : %s (%d bytes)\n", intToSuffixedStr(self.wave_info.bytes_per_sec), self.wave_info.bytes_per_sec)
	msg += fmt.Sprintf("  Sample size                    : %d bits (%d bytes)\n", self.wave_info.bits_per_sample, self.wave_info.bytes_per_sample)
	// Computed values:
	msg += fmt.Sprintf("  Total samples                  : %d\n", self.wave_info.num_samples)
	msg += fmt.Sprintf("  Sound size                     : %s (%d bytes)\n", intToSuffixedStr(self.wave_info.data_bloc_size), self.wave_info.data_bloc_size)
	msg += fmt.Sprintf("  Sound duration                 : %v\n", self.wave_info.sound_duration)
	//
	msg += fmt.Sprintf("\nHiding informations\n")
	msg += fmt.Sprintf("===================\n")
	msg += fmt.Sprintf("  Density                        : %d bits per sample\n", gd.density)
	msg += fmt.Sprintf("    Samples for hide one byte    : %d\n", self.samples_for_one_byte)
	msg += fmt.Sprintf("    Sample alteration @15%% dyn.  : %.5f%% max.\n", max_dist)
	msg += fmt.Sprintf("  Max samples offset             : %d\n", self.wave_max_offset)
	msg += fmt.Sprintf("    User samples offset          : %d (%v)\n", self.wave_start_offset, hidden_start_time)
	msg += fmt.Sprintf("    Max payload size             : %s (%d bytes)\n", intToSuffixedStr(self.payload_max_size), self.payload_max_size)

	fmt.Fprintln(output, msg)
	return nil
}

// HidePayload do it's job ;)
func (self *wave_handler_struct) HidePayload(sample_offset uint32) (err error) {
	var (
		payload_bloc_size  = self.bloc_size
		samples_bloc_size  = payload_bloc_size * self.samples_for_one_byte * self.wave_info.bytes_per_sample
		payload_bloc       = make(PayloadBloc, payload_bloc_size)
		samples_bloc       = make(SamplesBloc, samples_bloc_size)
		byte_offset        = sample_offset * self.wave_info.bytes_per_sample // Offset is expressed as sample count
		s_pos              = int64(self.wave_first_sample_pos + byte_offset)
		payload_bytes_read int
		samples_bytes_read int
	)

	// Go to desired samples
	if s_pos, err = self.wave_file.Seek(int64(s_pos), os.SEEK_SET); err != nil {
		return err
	}

	//-------------- Write len of payload
	buff := new(bytes.Buffer)
	err = binary.Write(buff, binary.LittleEndian, uint32(self.payload_file_size))
	steg_bloc := make(SamplesBloc, uint32(buff.Len())*self.samples_for_one_byte*self.wave_info.bytes_per_sample)
	var size_bloc PayloadBloc = buff.Bytes()
	if samples_bytes_read, err = self.wave_file.Read(steg_bloc[0:]); err != nil {
		return err
	}
	s_pos, err = self.wave_file.Seek(int64(-samples_bytes_read), os.SEEK_CUR)
	self.StegBloc(&size_bloc, &steg_bloc)
	if _, err = self.wave_file.Write(steg_bloc[0:samples_bytes_read]); err != nil {
		return err
	}
	//--------------

	// Read first payload bloc
	if payload_bytes_read, err = self.payload_file.Read(payload_bloc[0:]); err != nil {
		if err != io.EOF {
			return err
		}
	}

	// Read first samples bloc
	if samples_bytes_read, err = self.wave_file.Read(samples_bloc[0:]); err != nil {
		if err != io.EOF {
			return err
		}
	}

	// Loop until payload EOF
	for payload_bytes_read != 0 {
		// Steg
		self.StegBloc(&payload_bloc, &samples_bloc)

		// Write
		if s_pos, err = self.wave_file.Seek(int64(-samples_bytes_read), os.SEEK_CUR); err != nil {
			return err
		}

		if _, err = self.wave_file.Write(samples_bloc[0:samples_bytes_read]); err != nil {
			return err
		}

		s_pos, err = self.wave_file.Seek(0, os.SEEK_CUR)

		// Read next blocs
		if payload_bytes_read, err = self.payload_file.Read(payload_bloc[0:]); err != nil {
			if err != io.EOF {
				return err
			}
		}

		if payload_bytes_read > 0 {
			if samples_bytes_read, err = self.wave_file.Read(samples_bloc[0:]); err != nil {
				if err != io.EOF {
					return err
				}
			}
		}
	}
	self.wave_file.Sync()

	return nil
}

func (self *wave_handler_struct) ExtractPayload(offset uint32, output *os.File) (err error) {
	var (
		payload_bloc_size  = self.bloc_size
		samples_bloc_size  = payload_bloc_size * self.samples_for_one_byte * self.wave_info.bytes_per_sample
		payload_bloc       = make(PayloadBloc, payload_bloc_size)
		samples_bloc       = make(SamplesBloc, samples_bloc_size)
		byte_to_read       uint32
		wave_file          = self.wave_file
		p_size             uint32
		samples_bytes_read int
	)

	// Jump to beginning of hidden data.
	offset *= uint32(self.wave_info.bytes_per_sample)
	if _, err = wave_file.Seek(int64(self.wave_first_sample_pos+offset), os.SEEK_SET); err != nil {
		return err
	}

	// Get size of hidden data to extract (in bytes)
	payload := make(PayloadBloc, 4) // uint32
	samples := make(SamplesBloc, 4*self.samples_for_one_byte*self.wave_info.bytes_per_sample)
	buff := bytes.NewBuffer(payload)
	if samples_bytes_read, err = self.wave_file.Read(samples[0:]); err != nil {
		return err
	}
	self.UnstegBloc(&samples, &payload)
	if err = binary.Read(buff, binary.LittleEndian, &p_size); err != nil {
		return err
	}
	//fmt.Fprintf(os.Stderr, "Read payload of size %d (--%d--)\n", p_size, samples_bytes_read)

	// Check Consistency of data_size
	if p_size > self.payload_max_size {
		return errors.New(fmt.Sprintf("Consistency error. "+
			"Size of data to extract (%s) is bigger than maximum (%s) payload. Maybe a wrong offset ?",
			intToSuffixedStr(p_size), intToSuffixedStr(self.payload_max_size)))
	}

	byte_to_read = (p_size * self.samples_for_one_byte * self.wave_info.bytes_per_sample)
	for byte_to_read != 0 {

		if byte_to_read < uint32(len(samples_bloc)) {
			p_len := byte_to_read / self.wave_info.bytes_per_sample / self.samples_for_one_byte
			samples_bloc = samples_bloc[0:byte_to_read]
			payload_bloc = payload_bloc[0:p_len]
		}

		if samples_bytes_read, err = self.wave_file.Read(samples_bloc[0:]); err != nil {
			if err != io.EOF {
				return err
			}
		}

		byte_to_read -= uint32(samples_bytes_read)

		self.UnstegBloc(&samples_bloc, &payload_bloc)
		output.Write(payload_bloc[0:])

	}

	return nil
}

// parseHeaders parse the file headers and collect informations.
func (self *wave_handler_struct) parseHeaders() (err error) {
	/*
	 * http://www.lightlink.com/tjweber/StripWav/WAVE.html#WAVE
	 *
	 * The *canonical* WAVE format starts with the RIFF header:
	 * http://ccrma.stanford.edu/courses/422/projects/WaveFormat/
	 */

	var (
		chunk            = []byte{0, 0, 0, 0}
		wave_file        = self.wave_file
		v32              uint32
		parse_next_chunk = true
	)

	// RIFF chunk
	if err = binary.Read(wave_file, binary.LittleEndian, &chunk); err != nil {
		return err
	}

	if string(chunk[:4]) != "RIFF" {
		return errors.New("Not a RIFF file")
	}

	// RIFF chunk size
	if err = binary.Read(wave_file, binary.LittleEndian, &v32); err != nil {
		return err
	}

	if v32+8 != uint32(self.wave_file_size) {
		return errors.New("Damaged file. Chunk size != file size.")
	}

	// RIFF chunk format
	if err = binary.Read(wave_file, binary.LittleEndian, &chunk); err != nil {
		return err
	}

	if string(chunk[:4]) != "WAVE" {
		return errors.New("Not a WAVE file")
	}

	for parse_next_chunk {
		// Read next chunkID
		if err = binary.Read(wave_file, binary.BigEndian, &chunk); err != nil {
			return err
		}
		// and it's size in bytes
		if err = binary.Read(wave_file, binary.LittleEndian, &v32); err != nil {
			return err
		}
		chunklen := v32

		switch string(chunk[:4]) {
		case "fmt ":
			self.wave_info.canonical = chunklen == 16 // canonical format if chunklen == 16
			if err = self.parseChunkFmt(); err != nil {
				return err
			}
		case "data":
			parse_next_chunk = false
			size, _ := wave_file.Seek(0, os.SEEK_CUR)
			self.wave_first_sample_pos = uint32(size)
			self.wave_info.data_bloc_size = uint32(chunklen)
		default:
			//fmt.Fprintf(os.Stderr, "Skip unused chunk \"%s\" (%d bytes).\n", chunk, v32)
			if _, err = wave_file.Seek(int64(chunklen), os.SEEK_CUR); err != nil {
				return err
			}
		}
	}

	// Is audio supported ?	 
	if self.wave_info.audio_format != 1 {
		return errors.New("Only PCM (not compressed) format is supported.")
	}

	// Compute some useful values
	self.wave_info.bytes_per_sample = self.wave_info.bits_per_sample >> 3
	self.wave_info.num_samples = self.wave_info.data_bloc_size / self.wave_info.bytes_per_sample
	self.wave_info.sound_duration = time.Duration(float64(self.wave_info.data_bloc_size)/float64(self.wave_info.bytes_per_sec)) * time.Second

	self.wave_start_offset = gd.offset
	self.wave_start_offset_in_bytes = gd.offset * self.wave_info.bytes_per_sample

	self.samples_for_one_byte = 8 / gd.density

	// Samples needed to store at least ONE payload bloc + size of payload
	val := (4 * self.samples_for_one_byte) + (self.bloc_size * self.samples_for_one_byte)
	self.wave_max_offset = self.wave_info.num_samples - val

	payload_samples_space := self.wave_info.num_samples - self.wave_start_offset
	self.payload_max_size = payload_samples_space / self.samples_for_one_byte

	if self.wave_start_offset >= self.wave_max_offset {
		return errors.New("Offset is to big! Retry with bigger WAVE Audio file or reduce offset.")
	}

	return nil
}

// parseChunkFmt
func (self *wave_handler_struct) parseChunkFmt() (err error) {
	var (
		v16       uint16
		v32       uint32
		wave_file = self.wave_file
	)

	// <audio format> 1 = PCM not compressed    
	if err = binary.Read(wave_file, binary.LittleEndian, &v16); err != nil {
		return err
	}
	self.wave_info.audio_format = uint32(v16)

	// <# of channels>
	if err = binary.Read(wave_file, binary.LittleEndian, &v16); err != nil {
		return err
	}
	self.wave_info.num_channels = uint32(v16)

	// <Frequency>
	if err = binary.Read(wave_file, binary.LittleEndian, &v32); err != nil {
		return err
	}
	self.wave_info.sampling_frequency = v32

	// <Bytes per second>
	if err = binary.Read(wave_file, binary.LittleEndian, &v32); err != nil {
		return err
	}
	self.wave_info.bytes_per_sec = v32

	// <byte per bloc>
	if err = binary.Read(wave_file, binary.LittleEndian, &v16); err != nil {
		return err
	}
	self.wave_info.byte_per_bloc = uint32(v16)

	// <Bits per sample>
	if err = binary.Read(wave_file, binary.LittleEndian, &v16); err != nil {
		return err
	}
	self.wave_info.bits_per_sample = uint32(v16)

	if self.wave_info.canonical == false {
		// Get extra params size
		if err = binary.Read(wave_file, binary.LittleEndian, &v32); err != nil {
			return err
		}
		// Skip them
		if _, err = wave_file.Seek(int64(v32), os.SEEK_CUR); err != nil {
			return err
		}
	}

	return nil
}

// UnstegBloc extract payload. Payload bloc is write in place - slices are passed by reference
// Len of SampleBloc MUST be samples_for_one_byte aligned!
func (self *wave_handler_struct) UnstegBloc(samples *SamplesBloc, payload *PayloadBloc) (p_len uint32) {
	var (
		s_pos  uint32
		s_len  = uint32(len(*samples))
		s_skip = self.wave_info.bytes_per_sample
		s_mask = byte(1<<gd.density) - 1
		s      byte
	)

	var (
		p_pos   uint32
		p_shift = gd.density
		p       byte
	)

	for n := s_len / self.wave_info.bytes_per_sample / self.samples_for_one_byte; n != 0; n-- {

		// Loop over samples for extract ONE byte
		for i := uint32(0); i < self.samples_for_one_byte; i++ {
			// Sample is little endian ordered. LSB is first
			s = (*samples)[s_pos]
			// skip to next sample
			s_pos += s_skip
			// Make space for new bits
			p <<= p_shift
			// Filter sample LSBs and add it to recompose a complete byte. 
			p |= s & s_mask
		}

		// Store payload
		(*payload)[p_pos] = p
		p_pos++
	}

	return p_pos
}

// StegBloc hide payload in samples. Samples is modified in place because slices are passed by reference
func (self *wave_handler_struct) StegBloc(payload *PayloadBloc, samples *SamplesBloc) {
	// Payload vars
	var (
		p_pos   uint32
		p_len   = uint32(len(*payload))
		p_byte  byte
		p_shift = gd.density
	)

	// Samples vars
	var (
		s_pos   uint32
		s_byte  byte
		s_mask  byte = ^((1 << gd.density) - 1)
		s_skip       = self.wave_info.bytes_per_sample
		s_shift      = 8 - gd.density
	)

	for ; p_len != 0; p_len-- {
		// Read payload byte
		p_byte = (*payload)[p_pos]
		p_pos++

		//Steg with sample LSB byte
		for i := uint32(0); i < self.samples_for_one_byte; i++ {
			// Read sample LSB byte. Alway the first because of Little Endian order.
			s_byte = (*samples)[s_pos]

			// Steg
			s_byte &= s_mask
			s_byte |= p_byte >> s_shift
			p_byte <<= p_shift

			// Write
			(*samples)[s_pos] = s_byte

			// Jump to next sample
			s_pos += s_skip
		}
	}
}

// free allocated ressources
func (self *wave_handler_struct) Free() {
	if self.payload_file != nil {
		self.payload_file.Close()
	}

	if self.wave_file != nil {
		self.wave_file.Close()
	}

}

func main() {
	var wh = &wave_handler_struct{bloc_size: 4096}
	//	var dataf *os.File
	var err error

	// Read cmd line arguments
	parseArgs()
	defer wh.Free()

	switch {
	case gd.action == ACTION_HELP:
		show_usage()
		os.Exit(0)
	case gd.action == ACTION_VERSION:
		fmt.Println(APP + " (" + os.Args[0] + ") " + VERSION + ".")
		fmt.Println("Copyright (C) 2012 Stéphane Bunel.")
		fmt.Println("License: BSD style (included in source code).")
		os.Exit(0)
	case gd.action == ACTION_INFO:
		if err = wh.OpenWave(gd.wave_file, false); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open \"%s\": %s\n", gd.wave_file, err)
			break
		}

		if err = wh.PrintWAVInfo(os.Stdout); err != nil {
			log.Fatal(fmt.Sprintf("%s\n", err))
		}
		os.Exit(0)
	case gd.action == ACTION_EXTRACT:
		if err = wh.OpenWave(gd.wave_file, false); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open \"%s\": %s\n", gd.wave_file, err)
			break
		}

		if err = wh.ExtractPayload(uint32(gd.offset), os.Stdout); err != nil {
			log.Fatal(fmt.Sprintf("%s\n", err))
		}
		os.Exit(0)
	case gd.action == ACTION_HIDE:
		if err = wh.OpenWave(gd.wave_file, true); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open \"%s\": %s\n", gd.wave_file, err)
			break
		}

		if gd.density > wh.wave_info.bits_per_sample/2 {
			fmt.Fprintf(os.Stderr, "Density of %d is too high for sample size of %d bits.\n", gd.density,
				wh.wave_info.bits_per_sample)
			break
		}

		if err = wh.OpenPayload(gd.data_file); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open \"%s\": %s\n", gd.data_file, err)
			break
		}

		t0 := time.Now()
		fmt.Printf("Hiding \"%s\" inside \"%s\" ...\n", wh.payload_file_name, wh.wave_file_name)

		if err = wh.HidePayload(uint32(gd.offset)); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			break
		}

		duration := time.Now().Sub(t0)
		byte_writed := uint32(wh.payload_file_size) * wh.samples_for_one_byte * wh.wave_info.bytes_per_sample
		fmt.Printf("Read %s from \"%s\" and write %s to \"%s\" in %v (%s/s).\n",
			intToSuffixedStr(uint32(wh.payload_file_size)), wh.payload_file_name,
			intToSuffixedStr(byte_writed), wh.wave_file_name,
			duration, intToSuffixedStr(uint32(float64(byte_writed)/duration.Seconds())))
		os.Exit(0)
	}
	os.Exit(1)
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

	density := flag.Uint64("density", 4, "")
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
		fmt.Fprintln(os.Stderr, "Option --payload=<filename> is mandatory for this action.")
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
		"  --wave=<filename>    : Path to WAVE/PCM Audio file.\n"+
			"  --payload=<filename> : Path to file containing data to hide.\n"+
			"  --density=<integer>  : Must be 1, 2, 4 or 8 (default to 4).\n"+
			"  --offset=<integer>   : Must be > 0. This is one of your SECRET.\n")

	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  Get informations about capsule:")
	fmt.Fprintln(os.Stderr, "  $ steganoWAV --info --wave=boris24.2.wav --offset=5432\n")
	fmt.Fprintln(os.Stderr, "  Hide source code of steganoWAV:")
	fmt.Fprintln(os.Stderr, "  $ steganoWAV --hide --wave=boris24.2.wav --offset=5432 --payload=steganoWAV.go\n")
	fmt.Fprintln(os.Stderr, "  Extract source code to stdout:")
	fmt.Fprintln(os.Stderr, "  $ steganoWAV --extract --wave=boris24.2.wav --offset=5432\n")
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
