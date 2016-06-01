package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"time"
)

func main() {
	sleep := flag.Int("sleep", 0, "sleep seconds after output (default: 0)")
	flag.Parse()
	b, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}
	log.Printf("(%d) %q", len(b), b)
	time.Sleep(time.Duration(*sleep) * time.Second)
}
