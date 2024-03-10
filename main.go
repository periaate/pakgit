package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver"
)

const fName = "./pkgit.req"

var Req = &req{}

type req struct {
	TargetDir    string   `json:"targetDir"`
	Dependencies []source `json:"dependencies"`
}
type source struct {
	Semver string
	Hash   string
	Repo   string
}

func (r *req) Make() {
	if _, err := os.Stat(fName); os.IsNotExist(err) {
		f, err := os.Create("./pkgit.req")
		if err != nil {
			panic(err)
			// log.Fatalln("couldn't create file", err)
		}
		f.Close()
	}
}
func (r *req) ReadF() {
	b, err := os.ReadFile(fName)
	if err != nil {
		log.Fatalln(err)
	}

	err = json.Unmarshal(b, Req)
	if err != nil {
		log.Fatalln("failed to parse json, readReq")
	}
}

func (r *req) Dump() {
	b, err := json.Marshal(r)
	if err != nil {
		log.Fatalln("error parsing req to json", err)
	}

	err = os.WriteFile(fName, b, 0644)
	if err != nil {
		panic(err)
	}
}

func (s source) arg() string { return fmt.Sprintf("%s@%s", s.Repo, s.Semver) }

type tagged interface {
	semver() string
	hash() string
	url() string
}

type githubCommit struct {
	Sha string `json:"sha"`
	URL string `json:"url"`
}

type github struct {
	Name       string       `json:"name"`
	ZipballURL string       `json:"zipball_url"`
	TarballURL string       `json:"tarball_url"`
	Commit     githubCommit `json:"commit"`
	Node_id    string       `json:"node_id"`
}

func (gh github) semver() string { return semver.MustParse(gh.Name).String() }
func (gh github) hash() string   { return gh.Commit.Sha }
func (gh github) url() string    { return gh.ZipballURL }

func main() {
	os.Args = os.Args[1:]
	fmt.Println(len(os.Args), os.Args[0])
	switch len(os.Args) {
	case 0:
		log.Fatalln("too few arguments")
	case 1:
		switch os.Args[0] {
		case "init":
			initMod("pkgit")
		case "install":
			Req.ReadF()
			installReq()
		default:
			log.Fatalf("no such command \"%s\"\n", os.Args[0])
		}
	case 2:
		if os.Args[0] != "init" {
			mustHaveInit()
		}
		switch os.Args[0] {
		case "init":
			initMod(os.Args[1])
		case "get":
			Req.ReadF()
			getPkg(os.Args[1])
		default:
			log.Fatalf("no such command \"%s\"\n", os.Args[0])
		}
	}
}

func initMod(s ...string) {
	if len(s) > 0 {
		Req.TargetDir = s[0]
	}
	Req.Dependencies = make([]source, 0)
	Req.Dump()
	fmt.Println("successfully created pkgit.req file")
	os.Exit(0)
}

func installReq() {
	for _, dependency := range Req.Dependencies {
		getPkg(dependency.arg())
	}

}

func getPkg(arg string) {
	tar := parseSource(arg)
	src, err := getGitHub(tar)
	if err != nil {
		log.Fatalln(err)
	}

	if err = downloadAndExtractZip(src.url(), Req.TargetDir); err != nil {
		log.Fatalln(err)
	}

	Req.Dependencies = append(Req.Dependencies, source{src.semver(), src.hash(), arg})
	Req.Dump()
}

func mustHaveInit() {
	if _, err := os.Stat("./pkgit.req"); os.IsNotExist(err) {
		log.Fatalln("file ./pkgit.req does not exist, call init to use")
	}
}
func parseSource(s string) (res [3]string) {
	sar := strings.Split(s, "@")
	fmt.Println(s, len(sar), sar)
	switch {
	case len(sar) == 0:
		log.Fatalln("invalid URL: too short")
	case len(sar) == 2:
		res[2] = sar[1]
	case len(sar) > 2:
		log.Fatalln("invalid URL: too many @ signs")
	}

	sar = strings.Split(sar[0], "/")
	fmt.Println(len(sar), len(sar) < 3, sar)
	if len(sar) < 3 {
		log.Fatalln("invalid url", sar)
	}

	repo := sar[len(sar)-1]
	username := sar[len(sar)-2]

	res[0], res[1] = username, repo
	return
}

func getGitHub(target [3]string) (t tagged, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/tags", target[0], target[1])
	fmt.Println("requesting URL:", url)
	resp, err := http.Get(url)
	if err != nil {
		return
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	gh := []github{}

	err = json.Unmarshal(b, &gh)
	if err != nil {
		log.Fatalln("failed to parse json")
	}

	for _, ver := range gh {
		if len(target[2]) == 0 {
			return ver, nil
		}

		if ver.Name[1:] == target[2] {
			return ver, nil
		}
	}

	return nil, errors.New("no valid tagged repository found")
}

// chatgpt generated code
// TODO: refactor; change so that the folders (from github at least) don't get `-{hash here}` appended to the end
func downloadAndExtractZip(url, outputPath string) error {
	// Make the HTTP request to get the zip file
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the body into a byte array
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Create a reader out of the byte array
	zipReader, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
	if err != nil {
		return err
	}

	// Iterate through each file in the zip
	for _, zipFile := range zipReader.File {
		zipFileReader, err := zipFile.Open()
		if err != nil {
			return err
		}

		// Define the path where the file should be written
		path := filepath.Join(outputPath, zipFile.Name)

		// Check for Zip Slip (directory traversal) vulnerability
		if !strings.HasPrefix(path, filepath.Clean(outputPath)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		// Create directories if necessary
		if zipFile.FileInfo().IsDir() {
			os.MkdirAll(path, os.ModePerm)
		} else {
			if err = os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zipFile.Mode())
			if err != nil {
				zipFileReader.Close()
				return err
			}
			_, err = io.Copy(file, zipFileReader)
			file.Close()
			if err != nil {
				zipFileReader.Close()
				return err
			}
		}
		zipFileReader.Close()
	}

	return nil
}
