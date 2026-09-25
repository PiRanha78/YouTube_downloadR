package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	id3v2 "github.com/bogem/id3v2/v2"
)

func TestCropSquareJPEG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			c := color.RGBA{G: 255, A: 255}
			if x < 70 {
				c = color.RGBA{R: 255, A: 255}
			} else if x >= 250 {
				c = color.RGBA{B: 255, A: 255}
			}
			src.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	out, err := cropSquareJPEG(buf.Bytes(), 100)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 100 {
		t.Fatalf("size %dx%d", got.Bounds().Dx(), got.Bounds().Dy())
	}
	// A 180-es négyzet a 320 széles kép közepéről jön, a két szél színe kiesik.
	c := got.At(50, 50)
	r, g, b, _ := c.RGBA()
	if g < r || g < b {
		t.Fatalf("a közép nem a megtartott sáv: r=%d g=%d b=%d", r, g, b)
	}
}

func TestReplaceMP3Cover(t *testing.T) {
	ffmpeg := filepath.Join(os.Getenv("LOCALAPPDATA"), "YouTube_DownloadR", "runtime", "bundle-1.3.4", "ffmpeg.exe")
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe.exe")
	if _, err := os.Stat(ffmpeg); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	wide := filepath.Join(dir, "wide.jpg")
	mp3 := filepath.Join(dir, "a.mp3")
	src := image.NewRGBA(image.Rect(0, 0, 320, 180))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wide, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	silent := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "0.2", "-c:a", "libmp3lame", mp3)
	if out, err := silent.CombinedOutput(); err != nil {
		t.Fatalf("silent: %v %s", err, out)
	}
	embed := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-i", mp3, "-i", wide, "-map", "0:a:0", "-map", "1:v:0", "-c", "copy",
		"-id3v2_version", "3", "-disposition:v:0", "attached_pic", mp3+".in.mp3")
	if out, err := embed.CombinedOutput(); err != nil {
		t.Fatalf("embed: %v %s", err, out)
	}
	if err := os.Rename(mp3+".in.mp3", mp3); err != nil {
		t.Fatal(err)
	}
	if err := replaceMP3Cover(ffmpeg, mp3, 100); err != nil {
		t.Fatal(err)
	}
	probe := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=p=0", mp3)
	out, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("probe: %v %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "100,100" {
		t.Fatalf("beágyazott méret %q", out)
	}
	tag, err := id3v2.Open(mp3, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tag.Close()
	frames := tag.GetFrames(tag.CommonID("Attached picture"))
	if len(frames) != 1 {
		t.Fatalf("borítókeretek: %d", len(frames))
	}
	pic := frames[0].(id3v2.PictureFrame)
	got, err := jpeg.Decode(bytes.NewReader(pic.Picture))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 100 {
		t.Fatalf("APIC %dx%d", got.Bounds().Dx(), got.Bounds().Dy())
	}
}
