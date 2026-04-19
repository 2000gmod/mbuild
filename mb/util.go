package mb

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

func generateExample(v any) any {
	return genExample(reflect.ValueOf(v), make(map[reflect.Type]bool)).Interface()
}

func genExample(val reflect.Value, seen map[reflect.Type]bool) reflect.Value {
	// Dereference pointers, but allocate new ones if nil
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		elem := genExample(val.Elem(), seen)
		val.Elem().Set(elem)
		return val
	}

	t := val.Type()
	switch val.Kind() {
	case reflect.Struct:
		// Avoid infinite recursion on cyclic references
		if seen[t] {
			return val
		}
		seen[t] = true
		defer delete(seen, t)

		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			tag := t.Field(i).Tag.Get("json")
			if tag == "-" {
				continue
			}
			// Skip fields with omitempty? We include them anyway for the example.
			field.Set(genExample(field, seen))
		}
		return val

	case reflect.Slice, reflect.Array:
		if val.Len() == 0 && val.CanSet() {
			// Create a slice with one example element
			elemType := t.Elem()
			newSlice := reflect.MakeSlice(t, 1, 1)
			exampleElem := genExample(reflect.New(elemType).Elem(), seen)
			newSlice.Index(0).Set(exampleElem)
			val.Set(newSlice)
		} else {
			for i := 0; i < val.Len(); i++ {
				val.Index(i).Set(genExample(val.Index(i), seen))
			}
		}
		return val

	case reflect.Map:
		if val.IsNil() && val.CanSet() {
			// Create a map with one example entry
			keyType := t.Key()
			elemType := t.Elem()
			if keyType.Kind() == reflect.String {
				newMap := reflect.MakeMap(t)
				exampleKey := reflect.ValueOf("example")
				exampleValue := genExample(reflect.New(elemType).Elem(), seen)
				newMap.SetMapIndex(exampleKey, exampleValue)
				val.Set(newMap)
			} // else skip non‑string keys for simplicity
		} else {
			iter := val.MapRange()
			for iter.Next() {
				k := iter.Key()
				v := iter.Value()
				newVal := genExample(v, seen)
				val.SetMapIndex(k, newVal)
			}
		}
		return val

	case reflect.String:
		if val.String() == "" && val.CanSet() {
			val.SetString("example")
		}
		return val

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val.Int() == 0 && val.CanSet() {
			val.SetInt(42)
		}
		return val

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if val.Uint() == 0 && val.CanSet() {
			val.SetUint(42)
		}
		return val

	case reflect.Float32, reflect.Float64:
		if val.Float() == 0 && val.CanSet() {
			val.SetFloat(3.14)
		}
		return val

	case reflect.Bool:
		if val.CanSet() {
			val.SetBool(true)
		}
		return val

	default:
		// Interfaces, chan, func – leave as zero value
		return val
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir() // Path exists, check if it's a directory
	}
	if errors.Is(err, os.ErrNotExist) {
		return false // Path does not exist
	}
	return false // Other error (e.g., permission denied)
}

// IsAnySourceNewer returns true if any source (or any file inside source directories)
// has a modification time newer than the target file. If the target does not exist,
// it returns true (considered out-of-date). Non-existent sources are skipped.
func isAnySourceNewer(targetPath string, sourcePaths []string) (bool, error) {
	// Get target modification time
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Target missing -> treat as outdated (sources are "newer")
			return true, nil
		}
		return false, err
	}
	targetMod := targetInfo.ModTime()

	// Check each source
	for _, src := range sourcePaths {
		newer, err := isAnyNewerThan(src, targetMod)
		if err != nil {
			return false, err
		}
		if newer {
			return true, nil
		}
	}
	return false, nil
}

// isAnyNewerThan checks if the given path (file or directory) contains any file
// with modification time after the given reference time.
func isAnyNewerThan(path string, refTime time.Time) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		// Skip missing sources (or handle as needed)
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if !info.IsDir() {
		// Single file: compare its mod time
		return info.ModTime().After(refTime), nil
	}

	// Directory: walk recursively
	var newer bool
	err = filepath.Walk(path, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip the directory entry itself, only check files
		if info.IsDir() {
			return nil
		}
		if info.ModTime().After(refTime) {
			newer = true
			return filepath.SkipDir // stop walking
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return newer, nil
}

func combineReaders(stdout, stderr io.Reader) io.Reader {
	pr, pw := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(2)

	// Copy stdout
	go func() {
		defer wg.Done()
		_, err := io.Copy(pw, stdout)
		if err != nil {
			// Propagate the error to the pipe reader
			pw.CloseWithError(err)
		}
	}()

	// Copy stderr
	go func() {
		defer wg.Done()
		_, err := io.Copy(pw, stderr)
		if err != nil {
			pw.CloseWithError(err)
		}
	}()

	// Close the pipe writer once both copy operations finish
	go func() {
		wg.Wait()
		pw.Close()
	}()

	return pr
}

// PrefixConfig holds the optional prefixes for stdout and stderr lines.
type PrefixConfig struct {
	StdoutPrefix string
	StderrPrefix string
}

// combineReadersWithPrefix merges stdout and stderr into a single io.Reader.
// Each line from stdout is prefixed with cfg.StdoutPrefix, and each line from
// stderr is prefixed with cfg.StderrPrefix. The combined output is streamed
// in real time as data arrives.
//
// If a prefix is empty, no prefix is added for that stream.
// Lines are defined by newline ('\n') characters. If the last line of a stream
// is not terminated by a newline, it is still written with the prefix before EOF.
func combineReadersWithPrefix(stdout, stderr io.Reader, cfg PrefixConfig) io.Reader {
	pr, pw := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(2)

	// processStream reads lines from r, prefixes them, and writes to w.
	processStream := func(r io.Reader, prefix string) {
		defer wg.Done()
		br := bufio.NewReader(r)
		var writeErr error

		for writeErr == nil {
			line, err := br.ReadBytes('\n')
			if len(line) > 0 {
				// Write prefix + line (line already includes the newline if present)
				if prefix != "" {
					if _, writeErr = pw.Write([]byte(prefix)); writeErr != nil {
						break
					}
				}
				if _, writeErr = pw.Write(line); writeErr != nil {
					break
				}
			}
			if err != nil {
				if err != io.EOF {
					// Propagate read error to the pipe reader
					pw.CloseWithError(err)
					return
				}
				// EOF: flush any remaining partial line
				if len(line) > 0 {
					// No trailing newline in the last chunk
					if prefix != "" {
						if _, writeErr = pw.Write([]byte(prefix)); writeErr != nil {
							break
						}
					}
					if _, writeErr = pw.Write(line); writeErr != nil {
						break
					}
				}
				break
			}
		}

		// If a write error occurred, close the pipe with that error
		if writeErr != nil {
			pw.CloseWithError(writeErr)
		}
	}

	go processStream(stdout, cfg.StdoutPrefix)
	go processStream(stderr, cfg.StderrPrefix)

	// Close the pipe writer when both streams are fully processed
	go func() {
		wg.Wait()
		pw.Close()
	}()

	return pr
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
