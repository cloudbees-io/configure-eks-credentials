package main

import (
	"log"

	"github.com/cloudbees-io/configure-eks-credentials/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
