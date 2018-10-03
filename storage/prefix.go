package storage

import (
	"bytes"

	dbm "github.com/tendermint/tendermint/libs/db"
	"github.com/tmthrgd/go-hex"
)

type Prefix []byte

func NewPrefix(p string) Prefix {
	return Prefix(p)
}

func (p Prefix) Key(key []byte) []byte {
	// Avoid any unintended memory sharing between keys
	return append(p[:len(p):len(p)], key...)
}

func (p Prefix) Suffix(key []byte) []byte {
	return key[len(p):]
}

// Get the lexicographical sibling above this prefix (i.e. the fixed length integer plus one)
func (p Prefix) Above() []byte {
	for i := len(p) - 1; i >= 0; i-- {
		c := p[i]
		if c < 0xff {
			inc := make([]byte, i+1)
			copy(inc, p)
			inc[i]++
			return inc
		}
	}
	return nil
}

// Get the lexicographical sibling below this prefix (i.e. the fixed length integer minus one)
func (p Prefix) Below() []byte {
	for i := len(p) - 1; i >= 0; i-- {
		c := p[i]
		if c > 0x00 {
			inc := make([]byte, i+1)
			copy(inc, p)
			inc[i]--
			return inc
		}
	}
	return nil
}

func (p Prefix) Iterator(iteratorFn func(start, end []byte) dbm.Iterator, start, end []byte) KVIterator {
	var pstart, pend []byte = p.Key(start), nil

	if end == nil {
		pend = p.Above()
	} else {
		pend = p.Key(end)
	}
	return &prefixIterator{
		start:  start,
		end:    end,
		prefix: p,
		source: iteratorFn(pstart, pend),
	}
}

func (p Prefix) ReverseIterator(iteratorFn func(start, end []byte) dbm.Iterator, start, end []byte) KVIterator {
	// Note because of the inclusive start, exclusive end on underlying iterator
	// To get inclusive start/end we have to handle the following:
	// 1012 above <- does not start with prefix (but included by underlying iterator)
	// 1011232
	// 1011 prefix
	// 1010111 <- does not start with prefix (but included by underlying iterator)
	// 1010 below
	var pstart, pend []byte
	above := p.Above()
	if start == nil {
		pstart = above
	} else {
		pstart = p.Key(start)
	}
	if end == nil {
		pend = p.Below()
	} else {
		pend = p.Key(end)
	}
	return &prefixIterator{
		start:  start,
		end:    end,
		prefix: p,
		// Skip 'above' if necessary
		source: skipOne(iteratorFn(pstart, pend), above),
	}
}

func (p Prefix) Iterable(source KVIterable) KVIterable {
	return &prefixIterable{
		prefix: p,
		source: source,
	}
}

type prefixIterable struct {
	prefix Prefix
	source KVIterable
}

func (pi *prefixIterable) Iterator(start, end []byte) KVIterator {
	return pi.prefix.Iterator(pi.source.Iterator, start, end)
}

func (pi *prefixIterable) ReverseIterator(start, end []byte) KVIterator {
	return pi.prefix.ReverseIterator(pi.source.ReverseIterator, start, end)
}

func (p Prefix) Store(source KVStore) KVStore {
	return &prefixKVStore{
		prefix: p,
		source: source,
	}
}

func (p Prefix) Length() int {
	return len(p)
}

func (p Prefix) String() string {
	return string(p)
}

func (p Prefix) HexString() string {
	return hex.EncodeUpperToString(p)
}

type prefixIterator struct {
	prefix  Prefix
	source  dbm.Iterator
	start   []byte
	end     []byte
	invalid bool
}

func (pi *prefixIterator) Domain() ([]byte, []byte) {
	return pi.start, pi.end
}

func (pi *prefixIterator) Valid() bool {
	pi.validate()
	return !pi.invalid && pi.source.Valid()
}

func (pi *prefixIterator) Next() {
	if pi.invalid {
		panic("prefixIterator.Next() called on invalid iterator")
	}
	pi.source.Next()
	pi.validate()
}

func (pi *prefixIterator) Key() []byte {
	if pi.invalid {
		panic("prefixIterator.Key() called on invalid iterator")
	}
	return pi.prefix.Suffix(pi.source.Key())
}

func (pi *prefixIterator) Value() []byte {
	if pi.invalid {
		panic("prefixIterator.Value() called on invalid iterator")
	}
	return pi.source.Value()
}

func (pi *prefixIterator) Close() {
	pi.source.Close()
}

func (pi *prefixIterator) validate() {
	if pi.invalid {
		return
	}
	sourceValid := pi.source.Valid()
	pi.invalid = !sourceValid || !bytes.HasPrefix(pi.source.Key(), pi.prefix)
	if pi.invalid {
		pi.Close()
	}
}

// If the first iterator item is skipKey, then
// skip it.
func skipOne(iterator dbm.Iterator, skipKey []byte) dbm.Iterator {
	if iterator.Valid() {
		if bytes.Equal(iterator.Key(), skipKey) {
			iterator.Next()
		}
	}
	return iterator
}

type prefixKVStore struct {
	prefix Prefix
	source KVStore
}

func (ps *prefixKVStore) Get(key []byte) []byte {
	return ps.source.Get(ps.prefix.Key(key))
}

func (ps *prefixKVStore) Has(key []byte) bool {
	return ps.source.Has(ps.prefix.Key(key))
}

func (ps *prefixKVStore) Set(key, value []byte) {
	ps.source.Set(ps.prefix.Key(key), value)
}

func (ps *prefixKVStore) Delete(key []byte) {
	ps.source.Delete(ps.prefix.Key(key))
}

func (ps *prefixKVStore) Iterator(start, end []byte) dbm.Iterator {
	return ps.prefix.Iterator(ps.source.Iterator, start, end)
}

func (ps *prefixKVStore) ReverseIterator(start, end []byte) dbm.Iterator {
	return ps.prefix.ReverseIterator(ps.source.ReverseIterator, start, end)
}
