package vedirect

import (
	"fmt"
	"log"

	"github.com/tarm/serial"
)

const (
	InChecksum = 1
	InFrame    = 2
	InLabel    = 3
	InValue    = 4
	WaitHeader = 5
)

type Block struct {
	Checksum int
	Fields   map[string]string
}

func (b Block) Validate() bool {
	return b.Checksum%256 == 0
}

type Stream struct {
	Device string
	//Port   *os.File
	Port  *serial.Port
	State int
}

type Streamer interface {
	Read() int
}

func NewStream(dev string) (Stream, error) {
	s := Stream{}
	s.Device = dev
	s.State = 0
	var err error

	c := &serial.Config{Name: s.Device, Baud: 19200}
	s.Port, err = serial.OpenPort(c)
	if err != nil {
		return s, err
	}
	fmt.Println("Stream initialized:", s)
	return s, nil
}

// Field format: <Newline><Field-Label><Tab><Field-Value>
// Last field in block will always be "Checksum".
// The value is a single byte, and the modulo 256 sum
// of all bytes in a block will equal 0 if there were
// no transmission errors.

func (s *Stream) ReadBlock() (Block, int) {
	var b = Block{}
	b.Fields = make(map[string]string)
	var frame_length int = 0
	var prev_state int
	var label = make([]byte, 0, 9)  // VE recommended buffer size.
	var value = make([]byte, 0, 33) // VE recommended buffer size.

	buf := make([]byte, 1)

	for {
		n, err := s.Port.Read(buf)
		if err != nil {
			log.Fatal(err)
		}

		str := string(buf[:n])
		var char byte = buf[0]

		// HEX mode is documented in BlueSolar-HEX-protocol-MPPT.pdf.
		// catch and ignore VE.Direct HEX frames from stream, otherwise
		// they mess up our checksum and we lose the current block.
		if char == ':' && s.State != InChecksum { // ":": beginning of frame
			prev_state = s.State // save state
			s.State = InFrame
			frame_length = 1
			continue
		}
		if s.State == InFrame {
			frame_length = frame_length + 1
			if str == "\n" { // end of frame
				s.State = prev_state // restore state
				//fmt.Printf("%d bytes HEX frame ignored\n", frame_length)
			}
			continue // ignore frame contents
		}

		// convert byte to integer and add to sum.
		b.Checksum += int(buf[0])

		// end of block. must process before byte evaluation.
		// checksum byte could have any value.
		if s.State == InChecksum {
			s.State = WaitHeader
			return b, b.Checksum % 256 // 0 on valid checksum
		}

		switch char {
		case 13: // "\r": beginning of field
			if s.State != WaitHeader { // avoid zero-valued entry on first run
				//  b.fields[label] = value
				b.Fields[string(label)] = string(value)
			}
			//label = ""
			//value = ""
			label = label[:0] // clear slice
			value = value[:0] // clear slice
			s.State = InLabel
			//continue

		case 10: // "\n": avoid appending \n to label
			//continue

		case 9: // "\t": label/value seperator
			if string(label) == "Checksum" {
				s.State = InChecksum
			} else {
				s.State = InValue
			}
			//continue

		default:
			if s.State == InLabel {
				//label += str
				label = append(label, buf[0])
			} else if s.State == InValue {
				//value += str
				value = append(value, buf[0])
			}
		}
	}
}
