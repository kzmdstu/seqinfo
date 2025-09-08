package main

import (
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/xuri/excelize/v2"
)

type Config struct {
	Fields []string
	Seq    SeqConfig
	Mov    MovConfig
}

type SeqConfig struct {
	Fields []Field
}

type MovConfig struct {
	Fields []Field
}

type Field struct {
	Name  string
	Value string
}

var FieldFuncs = template.FuncMap{
	"remap": func(path, from, to string) string {
		if !strings.HasPrefix(path, from) {
			return path
		}
		path = strings.Replace(path, from, to, 1)
		return path
	},
	"dirname": filepath.Dir,
	"abspath": filepath.Abs,
	"output": func(args ...string) (string, error) {
		if len(args) == 0 {
			return "", fmt.Errorf("command not specified")
		}
		cmd := args[0]
		args = args[1:]
		unsafeCmds := []string{"rm"}
		unsafe := false
		for _, c := range unsafeCmds {
			if cmd == c {
				unsafe = true
				break
			}
		}
		if unsafe {
			return "", fmt.Errorf("unsafe command: %v", cmd)
		}
		c := exec.Command(cmd, args...)
		out, err := c.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to execute: %v", string(out))
		}
		return string(out), nil
	},
}

type Sequence struct {
	Name  string
	Start string
	End   string
}

func (s *Sequence) FirstFile() string {
	return strings.Replace(s.Name, "{{$.Frame}}", s.Start, -1)
}

func (s *Sequence) LastFile() string {
	return strings.Replace(s.Name, "{{$.Frame}}", s.End, -1)
}

func (s *Sequence) Length() string {
	end, _ := strconv.Atoi(s.End)
	start, _ := strconv.Atoi(s.Start)
	return strconv.Itoa(end - start + 1)
}

var ReSplitSeqName = regexp.MustCompile(`(.*\D)?(\d+)(.*?)$`)

type Table struct {
	sync.Mutex
	Cells [][]string
}

func NewTable(i, j int) *Table {
	cells := make([][]string, i)
	for i := range cells {
		cells[i] = make([]string, j)
	}
	return &Table{Cells: cells}
}

func main() {
	// Do not print time in logs.
	log.SetFlags(0)
	// Parse Flags
	var (
		configFlag  string
		imgExtsFlag string
		movExtsFlag string
		sepFlag     string
		verboseFlag bool
		writeFlag   bool
		outputFlag  string
	)
	config := "config.toml"
	configHelp := "path of config file"
	envConfig := os.Getenv("SEQINFO_CONFIG")
	if envConfig != "" {
		config = envConfig
		configHelp += ", default inherited from SEQINFO_CONFIG environment variable"
	}
	flag.StringVar(&configFlag, "config", config, configHelp)
	flag.StringVar(&imgExtsFlag, "img-exts", "dpx,exr", "image extensions")
	flag.StringVar(&movExtsFlag, "mov-exts", "mov,mp4", "mov extensions")
	flag.StringVar(&sepFlag, "sep", "\t", "fields will be separated by this value when printed")
	flag.BoolVar(&verboseFlag, "v", false, "print errors from value calculation")
	flag.BoolVar(&writeFlag, "w", false, "write to excel file. will print instead when it is false.")
	flag.StringVar(&outputFlag, "o", "seqinfo_output.xlsx", "excel file path to be written. no-op if -w flag is off. existing file will be overrided.")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		log.Print(filepath.Base(os.Args[0]) + " [args...] searchroot")
		flag.PrintDefaults()
		return
	}
	if outputFlag == "" {
		// Cannot write, print instead.
		writeFlag = false
	}
	searchRoot := filepath.Clean(args[0])
	if configFlag == "" {
		log.Print(filepath.Base(os.Args[0]) + " [args...] searchroot")
		flag.PrintDefaults()
		return
	}
	cfg := &Config{}
	_, err := toml.DecodeFile(configFlag, cfg)
	if err != nil {
		log.Fatalf("could not decode config file (toml): %v", err)
	}
	fieldIdx := make(map[string]int)
	for i, field := range cfg.Fields {
		fieldIdx[field] = i
	}
	imgExts := strings.Split(imgExtsFlag, ",")
	movExts := strings.Split(movExtsFlag, ",")
	// Generate a template for each label.
	seqTmpl := make(map[string]*template.Template)
	for _, field := range cfg.Seq.Fields {
		t := template.New("t").Funcs(FieldFuncs)
		t, err = t.Parse(field.Value)
		if err != nil {
			log.Fatal(err)
		}
		seqTmpl[field.Name] = t
	}
	movTmpl := make(map[string]*template.Template)
	for _, field := range cfg.Mov.Fields {
		t := template.New("t").Funcs(FieldFuncs)
		t, err = t.Parse(field.Value)
		if err != nil {
			log.Fatal(err)
		}
		movTmpl[field.Name] = t
	}
	// Find sequences in the search root.
	seqs := make([]*Sequence, 0)
	movs := make([]*Mov, 0)
	err = filepath.WalkDir(searchRoot, func(path string, ent fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%v: %v", err, path)
		}
		if ent.IsDir() {
			return nil
		}
		path = filepath.Clean(path)
		ext := filepath.Ext(path)
		if ext == "" {
			return nil
		}
		ext = ext[1:] // remove .
		foundImg := false
		for _, e := range imgExts {
			if e == ext {
				foundImg = true
			}
		}
		if foundImg {
			dir := filepath.Dir(path)
			name := filepath.Base(path)
			m := ReSplitSeqName.FindStringSubmatch(name)
			if m == nil {
				return nil
			}
			pre := m[1]
			frame := m[2]
			f, _ := strconv.Atoi(frame)
			post := m[3]
			seq := filepath.Join(dir, pre+"{{$.Frame}}"+post)
			if len(seqs) == 0 || seqs[len(seqs)-1].Name != seq {
				seqs = append(seqs, &Sequence{Name: seq, Start: frame, End: frame})
			} else {
				s := seqs[len(seqs)-1]
				start, _ := strconv.Atoi(s.Start)
				end, _ := strconv.Atoi(s.End)
				if f < start {
					s.Start = frame
				} else if f > end {
					s.End = frame
				}
			}
		}
		foundMov := false
		for _, e := range movExts {
			if e == ext {
				foundMov = true
			}
		}
		if foundMov {
			mov, err := parseMov(path, verboseFlag)
			if err != nil {
				log.Fatal(err)
			}
			movs = append(movs, mov)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("walk failed: %v", err)
	}
	// Prepare writing to an excel file, if needed.
	var f *excelize.File
	if writeFlag {
		f = excelize.NewFile()
	}
	table := NewTable(len(seqs)+len(movs)+1, len(cfg.Fields)) // +1 for label
	// labels
	for j, field := range cfg.Fields {
		table.Cells[0][j] = field
	}
	// values
	type execInfo struct {
		i, j  int
		field string
		seq   *Sequence
		mov   *Mov
	}
	ch := make(chan execInfo)
	nothing := make(chan bool)
	done := make(chan bool)
	numConcurrent := 8
	for i := 0; i < numConcurrent; i++ {
		go func() {
			// Concurrently process each cell.
			for {
				select {
				case ex := <-ch:
					if ex.seq == nil && ex.mov == nil {
						log.Fatalf("execInfo have neither seq or mov: %v", ex)
					}
					out := strings.Builder{}
					var err error
					if ex.seq != nil {
						err = seqTmpl[ex.field].Execute(&out, ex.seq)
					} else {
						err = movTmpl[ex.field].Execute(&out, ex.mov)
					}
					if err != nil {
						if verboseFlag {
							log.Printf("failed to execute: %v", err)
						}
						continue
					}
					val := strings.TrimSpace(out.String())
					table.Lock()
					table.Cells[ex.i+1][ex.j] = val
					table.Unlock()
				case <-nothing:
					done <- true
					return
				}
			}
		}()
	}
	go func() {
		n := 0
		for _, s := range seqs {
			for _, field := range cfg.Seq.Fields {
				ch <- execInfo{i: n, j: fieldIdx[field.Name], field: field.Name, seq: s}
			}
			n++
		}
		for _, m := range movs {
			for _, field := range cfg.Mov.Fields {
				ch <- execInfo{i: n, j: fieldIdx[field.Name], field: field.Name, mov: m}
			}
			n++
		}
		for i := 0; i < numConcurrent; i++ {
			nothing <- true
		}
	}()
	for i := 0; i < numConcurrent; i++ {
		<-done
	}
	// Write to the destination.
	for i, row := range table.Cells {
		for j, val := range row {
			if !writeFlag {
				if j != 0 {
					fmt.Print(sepFlag)
				}
				fmt.Print(val)
			} else {
				cell, err := excelize.CoordinatesToCellName(j+1, i+1)
				if err != nil {
					log.Fatal(err)
				}
				f.SetCellValue("Sheet1", cell, val)
			}
		}
		if !writeFlag {
			fmt.Print("\n")
		}
	}
	// Save the result as an excel file, if needed.
	if writeFlag {
		err := f.SaveAs(outputFlag)
		if err != nil {
			log.Print(err)
		}
	}
}
