package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/xuri/excelize/v2"
)

type Config struct {
	Fields []Field
}

type Field struct {
	Name  string
	Value string
}

var FieldFuncs = template.FuncMap{
	"dirname": func(path string) string {
		return filepath.Dir(path)
	},
	"output": func(args ...string) (string, error) {
		if len(args) == 0 {
			return "", fmt.Errorf("command not specified")
		}
		cmd := args[0]
		args = args[1:]
		safeCmds := []string{"oiiotool"}
		safe := false
		for _, c := range safeCmds {
			if cmd == c {
				safe = true
				break
			}
		}
		if !safe {
			return "", fmt.Errorf("unknown command: %v", cmd)
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

var ReSplitSeqName = regexp.MustCompile(`(.*\D)?(\d+)(.*?)$`)

func main() {
	// Do not print time in logs.
	log.SetFlags(0)
	// Parse Flags
	var (
		configFlag  string
		extsFlag    string
		sepFlag     string
		verboseFlag bool
		writeToFlag string
	)
	flag.StringVar(&configFlag, "config", "config.toml", "path of config file")
	flag.StringVar(&extsFlag, "exts", "dpx,exr", "meaningful extensions")
	flag.StringVar(&sepFlag, "sep", "\t", "fields will be separated by this value when printed")
	flag.BoolVar(&verboseFlag, "v", false, "print errors from value calculation")
	flag.StringVar(&writeToFlag, "w", "", "excel file path to be written. existing file will be overrided. when unset, it will print the results instead.")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		log.Print(filepath.Base(os.Args[0]) + " [args...] searchroot")
		flag.PrintDefaults()
		return
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
	extensions := strings.Split(extsFlag, ",")
	// Generate a template for each label.
	tmpl := make(map[string]*template.Template)
	for _, field := range cfg.Fields {
		t := template.New("t").Funcs(FieldFuncs)
		t, err = t.Parse(field.Value)
		if err != nil {
			log.Fatal(err)
		}
		tmpl[field.Name] = t
	}
	// Find sequences in the search root.
	seqs := make([]*Sequence, 0)
	err = filepath.Walk(searchRoot, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("%v: %v", err, path)
		}
		if fi.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == "" {
			return nil
		}
		ext = ext[1:] // remove .
		found := false
		for _, e := range extensions {
			if e == ext {
				found = true
			}
		}
		if !found {
			return nil
		}
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
		return nil
	})
	if err != nil {
		log.Fatalf("walk failed: %v", err)
	}
	// Prepare writing to an excel file, if needed.
	var f *excelize.File
	if writeToFlag != "" {
		f = excelize.NewFile()
	}
	// write writes (or prints) the cell values in a row.
	write := func() func([]interface{}) {
		i := 0
		return func(vals []interface{}) {
			for j, val := range vals {
				switch v := val.(type) {
				case error:
					if verboseFlag {
						// It will break the lines when it is printing. But that's OK.
						log.Print(v)
					}
				case string:
					if writeToFlag == "" {
						if j != 0 {
							fmt.Print(sepFlag)
						}
						fmt.Print(val)
					} else {
						cell, err := excelize.CoordinatesToCellName(1, j+1)
						if err != nil {
							log.Fatal(err)
						}
						f.SetCellValue("Sheet1", cell, val)
					}
				default:
					log.Fatal("invalid value of type: %t", v)
				}
			}
			if writeToFlag == "" {
				fmt.Print("\n")
			}
			i++
		}
	}()
	// Write labels and values.
	labels := make([]interface{}, 0, len(cfg.Fields))
	for _, field := range cfg.Fields {
		labels = append(labels, field.Name)
	}
	write(labels)
	var values []interface{}
	for _, s := range seqs {
		values = make([]interface{}, 0, len(cfg.Fields))
		for _, field := range cfg.Fields {
			out := strings.Builder{}
			err := tmpl[field.Name].Execute(&out, s)
			if err != nil {
				if verboseFlag {
					values = append(values, fmt.Errorf("failed to execute: %v", err))
					continue
				}
			}
			val := strings.TrimSpace(out.String())
			values = append(values, val)
		}
		write(values)
	}
	// Save the result as an excel file, if needed.
	if writeToFlag != "" {
		err := f.SaveAs(writeToFlag)
		if err != nil {
			log.Print(err)
		}
	}
}