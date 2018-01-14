package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/quhar/napigo"
)

var (
	lang = flag.String("language", "ENG", "Language in which subtitles should be downloaded, if subtitles in provided language are not found, Polish is used")
)

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Println("file name missing")
		flag.Usage()
		os.Exit(1)
	}
	n := napigo.New()
	for _, fname := range flag.Args() {
		fmt.Printf("Downloading subtitles for %q...\n", fname)
		if err := download(n, fname); err != nil {
			fmt.Println(err)
		}
	}
}

func download(n *napigo.Napi, fname string) error {
	s, err := n.Download(fname, *lang)
	if err != nil {
		return fmt.Errorf("failed to download subtitles for %q: %v", fname, err)
	}
	subFname, err := napigo.SubFileName(fname)
	if err != nil {
		return fmt.Errorf("failed to generate subtitles file name from %q: %v", fname, err)
	}
	fmt.Printf("Saving subtitles to: %s\n", subFname)
	if err := ioutil.WriteFile(subFname, []byte(s), 0666); err != nil {
		return fmt.Errorf("Failed to write subtitles to file %q: %v", fname, err)
	}
	return nil
}
