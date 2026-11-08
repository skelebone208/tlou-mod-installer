package main

// Small archives (steam.psarc, epic.psarc, ...) can't simply be unpacked: the
// game only offers some content (the Steam / Epic Extras skins) when the
// archive itself is present, and it ignores the loose copies. So after every
// install / uninstall the installer rebuilds those archives from the loose
// files - original entries plus whatever mods changed - with every block
// stored uncompressed. The untouched original stays as <name>.psarc.bak and
// "Restore original game" puts it back as before.

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// archives up to this size are kept as rebuilt archives after unpacking
const repackLimit = 256 << 20

// Repack writes a copy of the archive to dst. Each named entry's content comes
// from src(name) when that returns a readable file, otherwise from the archive.
func (a *PSARC) Repack(dst string, src func(name string) string) error {
	var h0 [32]byte
	if _, err := a.f.ReadAt(h0[:], 0); err != nil {
		return err
	}
	be := binary.BigEndian
	entrySize := int64(be.Uint32(h0[16:]))
	bs := a.blockSize
	if bs > 65536 {
		return fmt.Errorf("block size %d not supported", bs)
	}
	type part struct {
		e    *psarcEntry
		path string // loose file, or "" = from the archive
		size int64
	}
	parts := make([]part, len(a.entries))
	var nblocks int64
	for i := range a.entries {
		e := &a.entries[i]
		p := part{e: e, size: e.Size}
		// entry 0 is the archive's own name list: always the original (a loose
		// NamesLookUp.txt in the game folder belongs to another archive)
		if i > 0 && e.Name != "" {
			if lp := src(e.Name); lp != "" {
				if st, err := os.Stat(lp); err == nil && !st.IsDir() {
					p.path, p.size = lp, st.Size()
				}
			}
		}
		parts[i] = p
		nblocks += (p.size + bs - 1) / bs
	}
	count := int64(len(parts))
	hs := 32 + count*entrySize + 2*nblocks
	if hs > 1<<30 || nblocks > 1<<31 {
		return fmt.Errorf("archive too large to rebuild")
	}
	return writeAtomic(dst, func(w io.Writer) error {
		hdr := make([]byte, hs)
		copy(hdr, h0[:12])
		be.PutUint32(hdr[12:], uint32(hs))
		be.PutUint32(hdr[16:], uint32(entrySize))
		be.PutUint32(hdr[20:], uint32(count))
		be.PutUint32(hdr[24:], uint32(bs))
		copy(hdr[28:32], h0[28:32])
		off, blk := hs, int64(0)
		lens := hdr[32+count*entrySize:]
		for i, p := range parts {
			e := hdr[32+int64(i)*entrySize:]
			copy(e, p.e.Hash[:])
			be.PutUint32(e[16:], uint32(blk))
			e[20] = byte(p.size >> 32)
			be.PutUint32(e[21:], uint32(p.size))
			e[25] = byte(off >> 32)
			be.PutUint32(e[26:], uint32(off))
			for rem := p.size; rem > 0; rem -= bs {
				if rem < bs {
					be.PutUint16(lens[2*blk:], uint16(rem))
				} // a full block is stored as 0
				blk++
			}
			off += p.size
		}
		if _, err := w.Write(hdr); err != nil {
			return err
		}
		for _, p := range parts {
			var n int64
			var err error
			if p.path != "" {
				var f *os.File
				if f, err = os.Open(p.path); err != nil {
					return err
				}
				n, err = io.Copy(w, f)
				f.Close()
			} else {
				cw := &countWriter{w: w}
				err = a.extract(p.e, cw)
				n = cw.n
			}
			if err != nil {
				return err
			}
			if n != p.size {
				return fmt.Errorf("%s changed size while rebuilding", p.e.Name)
			}
		}
		return nil
	})
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// isSmallArchive says whether an unpacked archive is rebuilt instead of left renamed.
func (g *Game) isSmallArchive(base string) bool {
	st, err := os.Stat(filepath.Join(g.Main, base+".bak"))
	if err != nil {
		st, err = os.Stat(filepath.Join(g.Main, base))
	}
	return err == nil && st.Size() <= repackLimit
}

// syncSmallArchives rebuilds every unpacked small archive from its loose files,
// so the game finds the archive (with the mods' changes in it) where it expects it.
func (g *Game) syncSmallArchives(s *State, ui UI) error {
	for _, r := range s.Setup {
		bak := r.Archive + ".bak"
		if _, err := os.Stat(filepath.Join(g.Main, bak)); err != nil || !g.isSmallArchive(r.Archive) {
			continue
		}
		a, err := g.open(bak)
		if err != nil {
			return err
		}
		err = a.Repack(filepath.Join(g.Main, r.Archive), func(name string) string {
			return filepath.Join(g.Main, filepath.FromSlash(name))
		})
		a.Close()
		if err != nil {
			return fmt.Errorf("couldn't rebuild %s: %v", r.Archive, err)
		}
		ui.Log("  rebuilt " + r.Archive + " with the changes (the game reads this archive directly)")
	}
	return nil
}

// optionalMissing reports whether a mod file is marked optional in mod.json and
// this copy of the game doesn't have it (e.g. Epic-only files on a Steam copy).
func (m *Mod) optionalMissing(dest string, ix *GameIndex, main string) bool {
	ld := strings.ToLower(dest)
	opt := false
	for _, o := range m.Manifest.Optional {
		o = strings.ToLower(strings.Trim(strings.ReplaceAll(o, "\\", "/"), "/"))
		if ok, _ := filepath.Match(o, ld); ok || o == ld {
			opt = true
			break
		}
	}
	if !opt {
		return false
	}
	if _, ok := ix.where[ld]; ok {
		return false
	}
	if _, err := os.Stat(filepath.Join(main, filepath.FromSlash(dest))); err == nil {
		return false
	}
	return true
}
