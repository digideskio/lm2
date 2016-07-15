package lm2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
)

const BlockSize = 16 * 1024 // 16 kB
const HeaderSize = 8 + 4 + 4 + 4

// Block represents a set of key-value records.
type Block struct {
	id         uint32
	nextBlock  uint32
	flags      uint8
	numRecords uint8

	records []Record
}

// HeaderBlock represents the first block, which
// contains global metadata.
type HeaderBlock struct {
	// Last committed transaction
	lastTx int64
	// ID of the first block in the free list
	freeBlock uint32
	// Current block accepting writes
	currentDataBlock uint32
	// Last allocated block ID
	lastBlock uint32
}

type Record struct {
	// Offset within the block
	offset int64
	// Created transaction ID
	created int64
	// Deleted transaction ID
	deleted int64
	// Block ID of the previous record
	prev uint32
	// Block ID of the next record
	next uint32
	// Key
	key string
	// Value
	value string
}

type DB struct {
	file     *os.File
	fileSize int64
	// Memory-mapped data file
	mapped []byte

	lock sync.Mutex

	// Header block
	header HeaderBlock
}

// NewDB opens the file located at filepath and initializes
// a new lm2 data file.
func NewDB(filepath string) (*DB, error) {
	f, err := os.OpenFile(filepath, os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if stat.Size() < 2*BlockSize {
		f.Close()
		return nil, errors.New("lm2: file too small")
	}

	mapped, err := syscall.Mmap(int(f.Fd()), 0, int(stat.Size()),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, err
	}

	db := &DB{
		file:     f,
		fileSize: stat.Size(),
		mapped:   mapped,
		lock:     sync.Mutex{},

		header: HeaderBlock{
			lastTx:           0,
			freeBlock:        0,
			currentDataBlock: 1, // block 0 is the header block
			lastBlock:        0,
		},
	}

	err = db.writeHeader()
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() {
	syscall.Munmap(db.mapped)
	db.file.Close()
}

func (db *DB) writeHeader() error {
	buf := bytes.NewBuffer(make([]byte, 0, HeaderSize))
	err := binary.Write(buf, binary.LittleEndian, db.header)
	if err != nil {
		return err
	}
	copy(db.mapped, buf.Bytes())
	return nil
}

func (db *DB) readBlock(blockID uint32) (*Block, error) {
	r := bytes.NewReader(db.mapped)
	blockOffset := int64(blockID) * BlockSize
	r.Seek(blockOffset, 0)
	block := &Block{}
	var err error
	err = binary.Read(r, binary.LittleEndian, &block.id)
	if err != nil {
		return nil, err
	}
	if blockID != block.id {
		// Inconsistent block ID.
		return nil, errors.New("lm2: inconsistent block ID")
	}
	err = binary.Read(r, binary.LittleEndian, &block.nextBlock)
	if err != nil {
		return nil, err
	}
	err = binary.Read(r, binary.LittleEndian, &block.flags)
	if err != nil {
		return nil, err
	}
	err = binary.Read(r, binary.LittleEndian, &block.numRecords)
	if err != nil {
		return nil, err
	}

	// Consume the rest of the contents of this block
	offset, err := r.Seek(0, 1)
	if err != nil {
		return nil, err
	}
	nextBlockOffset := int64(blockID+1) * BlockSize
	recordBuf := bytes.NewBuffer(nil)
	_, err = io.CopyN(recordBuf, r, nextBlockOffset-offset)
	if err != nil {
		return nil, err
	}

	// Follow overflow blocks
	nextBlock := block.nextBlock
	for nextBlock > 0 {
		// TODO
		break
	}
	return block, nil
}
