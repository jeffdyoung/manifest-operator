package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
)

type ManifestListEntry struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
	Platform  struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
	} `json:"platform"`
}

type ManifestList struct {
	SchemaVersion int                 `json:"schemaVersion"`
	MediaType     string              `json:"mediaType"`
	Manifests     []ManifestListEntry `json:"manifests"`
}

type Image struct {
	ref  string
	arch string
	os   string
}

type AuthEntry struct {
	Auth string `json:"auth"`
}
type AuthFile struct {
	Auths map[string]AuthEntry `json:"auths"`
}

func readAuthFile(filePath string) (*AuthFile, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var authFile AuthFile
	err = json.Unmarshal(data, &authFile)
	if err != nil {
		return nil, err
	}

	return &authFile, nil
}

func getDockerAuthConfig(authFile *AuthFile, registry string) (*types.DockerAuthConfig, error) {
	entry, exists := authFile.Auths[registry]
	if !exists {
		return nil, fmt.Errorf("no auth entry for registry: %s", registry)
	}

	decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid auth entry format")
	}

	return &types.DockerAuthConfig{
		Username: parts[0],
		Password: parts[1],
	}, nil
}
func createMFList(images []Image, finalMFImage string, ctx context.Context, authConfig *types.DockerAuthConfig) {
	sysCtx := &types.SystemContext{DockerAuthConfig: authConfig}

	var manifestList ManifestList
	manifestList.SchemaVersion = 2
	manifestList.MediaType = "application/vnd.oci.image.index.v1+json"

	for _, image := range images {
		ref, err := docker.ParseReference("//" + image.ref)
		if err != nil {
			panic(err)
		}

		imgSrc, err := ref.NewImageSource(ctx, sysCtx)
		if err != nil {
			panic(err)
		}

		manifestBytes, _, err := imgSrc.GetManifest(ctx, nil)
		if err != nil {
			panic(err)
		}
		fmt.Println("Image ", image.ref)

		manifestDigest, err := manifest.Digest(manifestBytes)
		if err != nil {
			panic(err)
		}

		entry := ManifestListEntry{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Size:      int64(len(manifestBytes)),
			Digest:    manifestDigest.String(),
			Platform: struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			}{
				Architecture: image.arch,
				OS:           image.os,
			},
		}

		manifestList.Manifests = append(manifestList.Manifests, entry)
		imgSrc.Close()
	}

	manifestListJSON, err := json.Marshal(manifestList)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(manifestListJSON))

	ref, err := docker.ParseReference("//" + finalMFImage)
	if err != nil {
		panic(err)
	}

	dest, err := ref.NewImageDestination(ctx, sysCtx)
	if err != nil {
		panic(err)
	}

	// manifestListType := "application/vnd.docker.distribution.manifest.list.v2+json"
	err = dest.PutManifest(ctx, manifestListJSON, nil)

	if err != nil {
		panic(err)
	}

	fmt.Println("Manifest list image created successfully!")
}
func main() {
	ctx := context.Background()

	authFilePath := "/run/user/1000/containers/auth.json"
	authFile, err := readAuthFile(authFilePath)

	armImage := "quay.io/jeffdyoung/hello:arm"
	x86Image := "quay.io/jeffdyoung/hello:x86"

	finalMFImage := "quay.io/jeffdyoung/hello:mf"
	if err != nil {
		fmt.Println("Error reading authfile:", err)
		return
	}
	registry := "quay.io"
	authConfig, err := getDockerAuthConfig(authFile, registry)
	if err != nil {
		fmt.Println("Error getting auth config:", err)
		return
	}

	var ImageList []Image
	ImageList = append(ImageList, Image{armImage, "arm64", "linux"})
	ImageList = append(ImageList, Image{x86Image, "amd64", "linux"})
	createMFList(ImageList, finalMFImage, ctx, authConfig)
}
