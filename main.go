package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

const (
	nixCacheS3Base  = "https://nix-cache.s3.amazonaws.com"
	nixCacheCDNBase = "https://cache.nixos.org"
	nixCacheRegion  = "us-east-1"
	userAgent       = "grep-nixos-cache 1.0 (https://github.com/notashelf/grep-nixos-cache)"
)

var (
	needle       = flag.String("needle", "", "String to look for in the target Nix store paths.")
	path         = flag.String("path", "", "Single Nix store path that need to be checked (mostly for testing purposes).")
	paths        = flag.String("paths", "", "Filename containing a newline-separated list of Nix store paths that need to be checked.")
	hydraEvalURL = flag.String("hydra_eval_url", "", "Hydra eval URL to get all output Nix store paths from.")
	parallelism  = flag.Int("parallelism", 15, "Number of simultaneous store paths to process in flight.")
)

func getAwsRegion() (string, error) {
	sess := session.Must(session.NewSession())
	svc := ec2metadata.New(sess)
	region, err := svc.Region()
	if err != nil {
		return "", err
	}
	return region, nil
}

func collectOutputPaths() ([]string, error) {
	if *path != "" {
		return []string{*path}, nil
	} else if *paths != "" {
		file, err := os.Open(*paths)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		var lines []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		return lines, scanner.Err()
	} else if *hydraEvalURL != "" {
		resp, err := http.Get(*hydraEvalURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		if err != nil {
			return nil, err
		}

		var paths []string
		for _, v := range data["builds"].([]interface{}) {
			build := v.(map[string]interface{})
			for _, output := range build["outputs"].([]interface{}) {
				paths = append(paths, output.(string))
			}
		}
		return paths, nil
	} else {
		return []string{}, nil
	}
}

func fetchNarInfo(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return "", nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "URL: ") {
			return strings.TrimPrefix(line, "URL: "), nil
		}
	}
	return "", errors.New("Did not find a NAR URL key")
}

func fetchNar(narURL string) ([]byte, error) {
	resp, err := http.Get(narURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var reader io.Reader
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
	default:
		reader = resp.Body
	}

	return io.ReadAll(reader)
}

func findNeedleInNar(needle string, nar []byte) []string {
	var filesMatched []string
	for _, file := range strings.Split(string(nar), "\n") {
		if strings.Contains(file, needle) {
			filesMatched = append(filesMatched, file)
		}
	}
	return filesMatched
}

func main() {
	flag.Parse()

	var urlBase = nixCacheCDNBase
	paths, err := collectOutputPaths()
	if err != nil {
		log.Fatalf("Error collecting output paths: %v", err)
	}

	if len(paths) == 0 {
		log.Print("No paths to check, exiting")
		os.Exit(1)
	} else if len(paths) >= 50 {
		log.Print("More than 50 paths to check, ensuring that we run co-located with the Nix cache...")
		region, err := getAwsRegion()
		if err != nil || region != nixCacheRegion {
			log.Printf("To avoid unnecessary costs to the NixOS project, please run this program in the AWS %s region. Exiting.", nixCacheRegion)
			os.Exit(1)
		} else {
			urlBase = nixCacheS3Base
		}
	}

	wg := &sync.WaitGroup{}
	matches := make(chan string)

	for _, p := range paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			hash := strings.Split(p, "-")[0]
			narInfoURL := fmt.Sprintf("%s/%s.narinfo", urlBase, hash)
			narURL, err := fetchNarInfo(narInfoURL)
			if err != nil {
				log.Printf("Error fetching NAR info for path %s: %v", p, err)
				return
			}

			nar, err := fetchNar(narURL)
			if err != nil {
				log.Printf("Error fetching NAR for path %s: %v", p, err)
				return
			}

			filesMatched := findNeedleInNar(*needle, nar)
			for _, file := range filesMatched {
				matches <- fmt.Sprintf("Found in %s: %s", p, file)
			}
		}(p)
	}

	go func() {
		wg.Wait()
		close(matches)
	}()

	for match := range matches {
		fmt.Println(match)
	}
}
