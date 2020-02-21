package redirect

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type stringSet map[string]struct{}

/**
 * @return	true if `str' already in set previously
 *			false otherwise
 */
func (set *stringSet) Add(str string) bool {
	_, found := (*set)[str]
	(*set)[str] = struct{}{}
	return found
}

type Nameitem struct {
	// Domain name set for lookups
	names stringSet

	path string
	mtime time.Time
	size int64
}

func PathsToNameitems(paths []string) []Nameitem {
	items := make([]Nameitem, len(paths))
	for i, path := range paths {
		items[i].path = path
	}
	return items
}

type Namelist struct {
	sync.RWMutex

	// List of name items
	items []Nameitem

	// Time between two reload of a name item
	// All name items shared the same reload duration
	reload time.Duration
}

func (n *Namelist) parseNamelistCore(i int) {
	item := &n.items[i]

	file, err := os.Open(item.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File not exist already reported at setup stage
			log.Debugf("%v", err)
		} else {
			log.Warningf("%v", err)
		}
		return
	}
	defer Close(file)

	stat, err := file.Stat()
	if err == nil {
		n.RLock()
		mtime := item.mtime
		size := item.size
		n.RUnlock()

		if stat.ModTime() == mtime && stat.Size() == size {
			return
		}
	} else {
		// Proceed parsing anyway
		log.Warningf("%v", err)
	}

	log.Debugf("Parsing " + file.Name())
	names := n.parse(file)

	n.Lock()
	item.names = names
	item.mtime = stat.ModTime()
	item.size = stat.Size()
	n.Unlock()
}

func (n *Namelist) parse(r io.Reader) stringSet {
	names := make(stringSet)

	lines := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines++
		line := scanner.Bytes()
		if i := bytes.Index(line, []byte{'#'}); i >= 0 {
			line = line[0:i]
		}

		domain := strings.ToLower(string(line))
		if IsDomainName(domain) {
			// To reduce memory, we don't use full qualified name
			names.Add(strings.TrimSuffix(domain, "."))
			continue
		}

		f := bytes.FieldsFunc(line, func(r rune) bool {
			return r == '/'
		})

		if len(f) != 3 {
			continue
		}

		// Format: server=/DOMAIN/IP
		if string(f[0]) != "server=" {
			continue
		}

		domain = strings.ToLower(string(f[1]))
		if !IsDomainName(domain) {
			log.Warningf("%v isn't domain...", domain)
			continue
		}
		if net.ParseIP(string(f[2])) == nil {
			continue
		}

		names.Add(strings.TrimSuffix(domain, "."))
	}

	log.Debugf("Name added: %v / %v", len(names), lines)

	return names
}

func (n *Namelist) parseNamelist() {
	for i := range n.items {
		n.parseNamelistCore(i)
	}

	for _, item := range n.items {
		log.Debugf(">>> %v", item)
	}
}

