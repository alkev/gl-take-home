package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/alkev/gl_take_home/internal/vecmath"
)

var (
	snapshotMagic   = [8]byte{'V', 'E', 'C', 'S', 'T', 'O', 'R', 'E'}
	snapshotVersion = uint32(1)
	castagnoliTable = crc32.MakeTable(crc32.Castagnoli)
)

// ErrCorruptSnapshot indicates a snapshot file is structurally invalid.
var ErrCorruptSnapshot = errors.New("corrupt snapshot")

// Save writes a snapshot atomically to path (path.tmp then rename).
func (s *Store) Save(path string) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}

	h := crc32.New(castagnoliTable)
	w := io.MultiWriter(f, h)

	s.mu.RLock()
	n := uint64(s.n) //nolint:gosec // s.n is a non-negative int row count
	if _, err := w.Write(snapshotMagic[:]); err != nil {
		s.mu.RUnlock()
		cleanup()
		return err
	}
	var hdr [16]byte
	binary.LittleEndian.PutUint32(hdr[0:4], snapshotVersion)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(s.dim)) //nolint:gosec // dim is a small positive int
	binary.LittleEndian.PutUint64(hdr[8:16], n)
	if _, err := w.Write(hdr[:]); err != nil {
		s.mu.RUnlock()
		cleanup()
		return err
	}
	for i := 0; i < s.n; i++ {
		if _, err := w.Write(s.meta[i].UUID[:]); err != nil {
			s.mu.RUnlock()
			cleanup()
			return err
		}
		label := []byte(s.meta[i].Label)
		if len(label) > 0xFFFF {
			s.mu.RUnlock()
			cleanup()
			return fmt.Errorf("label too long at row %d", i)
		}
		var ll [2]byte
		binary.LittleEndian.PutUint16(ll[:], uint16(len(label))) //nolint:gosec // length bound-checked above
		if _, err := w.Write(ll[:]); err != nil {
			s.mu.RUnlock()
			cleanup()
			return err
		}
		if _, err := w.Write(label); err != nil {
			s.mu.RUnlock()
			cleanup()
			return err
		}
	}
	buf := make([]byte, s.dim*4)
	for i := 0; i < s.n; i++ {
		row := s.rowSlice(i)
		for j, v := range row {
			binary.LittleEndian.PutUint32(buf[j*4:(j+1)*4], math.Float32bits(v))
		}
		if _, err := w.Write(buf); err != nil {
			s.mu.RUnlock()
			cleanup()
			return err
		}
	}
	s.mu.RUnlock()

	var sumBuf [4]byte
	binary.LittleEndian.PutUint32(sumBuf[:], h.Sum32())
	if _, err := f.Write(sumBuf[:]); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads a snapshot from path and replaces the current store contents.
func (s *Store) Load(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(raw) < 8+16+4 {
		return fmt.Errorf("%w: file too short", ErrCorruptSnapshot)
	}
	gotCRC := binary.LittleEndian.Uint32(raw[len(raw)-4:])
	wantCRC := crc32.Checksum(raw[:len(raw)-4], castagnoliTable)
	if gotCRC != wantCRC {
		return fmt.Errorf("%w: CRC mismatch", ErrCorruptSnapshot)
	}

	body := raw[:len(raw)-4]
	if string(body[:8]) != string(snapshotMagic[:]) {
		return fmt.Errorf("%w: bad magic", ErrCorruptSnapshot)
	}
	ver := binary.LittleEndian.Uint32(body[8:12])
	if ver != snapshotVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrCorruptSnapshot, ver)
	}
	dim := int(binary.LittleEndian.Uint32(body[12:16]))
	countU := binary.LittleEndian.Uint64(body[16:24])
	if countU > uint64(1<<31) {
		return fmt.Errorf("%w: count too large: %d", ErrCorruptSnapshot, countU)
	}
	count := int(countU)
	if dim != s.dim {
		return fmt.Errorf("%w: dim %d != store dim %d", ErrCorruptSnapshot, dim, s.dim)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks = nil
	s.invNorms = s.invNorms[:0]
	s.meta = s.meta[:0]
	s.byUUID = make(map[uuid.UUID]int, count)
	s.byLabel = make(map[string]int, count)
	s.n = 0

	off := 24
	meta := make([]rowMeta, count)
	for i := 0; i < count; i++ {
		if off+16+2 > len(body) {
			return fmt.Errorf("%w: truncated label table", ErrCorruptSnapshot)
		}
		var id uuid.UUID
		copy(id[:], body[off:off+16])
		off += 16
		ll := int(binary.LittleEndian.Uint16(body[off : off+2]))
		off += 2
		if off+ll > len(body) {
			return fmt.Errorf("%w: truncated label", ErrCorruptSnapshot)
		}
		label := string(body[off : off+ll])
		off += ll
		meta[i] = rowMeta{UUID: id, Label: label}
	}
	need := count * dim * 4
	if off+need > len(body) {
		return fmt.Errorf("%w: truncated vectors", ErrCorruptSnapshot)
	}
	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		start := off + i*dim*4
		for j := 0; j < dim; j++ {
			b := body[start+j*4 : start+j*4+4]
			vec[j] = math.Float32frombits(binary.LittleEndian.Uint32(b))
		}
		row := s.appendRow(vec)
		s.invNorms = append(s.invNorms, vecmath.InvNorm(vec))
		s.meta = append(s.meta, meta[i])
		s.byUUID[meta[i].UUID] = row
		s.byLabel[strings.ToLower(meta[i].Label)] = row
	}
	return nil
}
