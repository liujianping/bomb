package filter

import (
	"regexp"
	"io"
	"os"
	"bufio"
	"strings"
)

type Replacement struct{
	N int
	Literal bool
	replaces map[*regexp.Regexp]chan string
}

func NewReplacement(n int, literal bool) *Replacement{
	return &Replacement{N:n, Literal:literal, replaces:make(map[*regexp.Regexp]chan string, 8)}
}

func (r *Replacement) Source(re *regexp.Regexp, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	ch := make(chan string, r.N)	
	r.replaces[re] = ch

	rd := bufio.NewReader(file)
	for i := 0; i < r.N; i++ {
		line, err := rd.ReadString('\n')
		if  err == io.EOF && len(line) == 0 {
			rd.Reset(file)
			continue
		}
		line = strings.TrimSpace(line)
		// ignore blank line
		if line == "" {
			continue
		}
		ch <- line
	}
	close(ch)
	return nil
}

func (r *Replacement) Do(args ...*string){
	for re, ch := range r.replaces{
		line := <- ch		
		for _, arg := range args {
			repl := *arg
			if r.Literal {
				repl = re.ReplaceAllLiteralString(repl, line)
			} else {
				repl = re.ReplaceAllString(repl, line)
			}
			*arg = repl
		}
	}	
}

