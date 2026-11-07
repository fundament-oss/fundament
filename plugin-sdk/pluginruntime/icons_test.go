package pluginruntime

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path d="M0 0h1v1z"/></svg>`

// testPNG is a real 1x1 PNG.
var testPNG = pngOfSize(1, 1)

func pngOfSize(width, height int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestValidateIcons_PNGAtMaxDimension(t *testing.T) {
	content := pngOfSize(MaxPNGDimension, MaxPNGDimension)
	require.LessOrEqual(t, len(content), MaxLogoBytes)

	got, err := ValidateIcons(iconDef(IconDigest(content), ""), fileSet(content))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got.Logo, "data:image/png;base64,"))
}

func TestValidateIcons_AllowsBuiltInNames(t *testing.T) {
	for _, name := range []string{"", "globe", "boxes-3", "shield-check-mark"} {
		_, err := ValidateIcons(iconDef(name, "", name), nil)
		require.NoError(t, err, name)
	}
}

func iconDef(logo, dark string, menu ...string) *PluginDefinition {
	def := &PluginDefinition{}
	def.Metadata.Icon = logo
	def.Metadata.IconDark = dark
	for _, m := range menu {
		def.Spec.Menu.Organization = append(def.Spec.Menu.Organization, MenuEntry{CRD: "x", Icon: m})
	}
	return def
}

func fileSet(contents ...[]byte) map[string][]byte {
	files := map[string][]byte{}
	for _, c := range contents {
		files[IconDigest(c)] = c
	}
	return files
}

func svgWithComment(size int) []byte {
	return []byte(strings.Replace(testSVG, "</svg>", "<!--"+strings.Repeat("a", size)+"--></svg>", 1))
}

func TestValidateIcons_Valid(t *testing.T) {
	dark := []byte(strings.Replace(testSVG, "M0 0", "M0 1", 1))
	m1 := []byte(strings.Replace(testSVG, "M0 0", "M0 2", 1))
	m2 := []byte(strings.Replace(testSVG, "M0 0", "M0 3", 1))
	def := iconDef(IconDigest([]byte(testSVG)), IconDigest(dark), IconDigest(m1), IconDigest(m2))

	got, err := ValidateIcons(def, fileSet([]byte(testSVG), dark, m1, m2))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got.Logo, "data:image/svg+xml;base64,"))
	assert.Equal(t, dataURI("image/svg+xml", []byte(testSVG)), got.Logo)
	assert.Equal(t, dataURI("image/svg+xml", dark), got.LogoDark)
	assert.Equal(t, map[string]string{
		IconDigest(m1): dataURI("image/svg+xml", m1),
		IconDigest(m2): dataURI("image/svg+xml", m2),
	}, got.MenuIcons)

	got, err = ValidateIcons(iconDef(IconDigest(testPNG), ""), fileSet(testPNG))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got.Logo, "data:image/png;base64,"))
}

func TestValidateIcons_NoCustomIcons(t *testing.T) {
	got, err := ValidateIcons(iconDef("globe", "", "boxes-3", "database"), nil)
	require.NoError(t, err)
	assert.Empty(t, got.Logo)
	assert.Empty(t, got.LogoDark)
	assert.NotNil(t, got.MenuIcons)
	assert.Empty(t, got.MenuIcons)
}

func TestValidateIcons_SharedFile(t *testing.T) {
	d := IconDigest([]byte(testSVG))
	got, err := ValidateIcons(iconDef("", "", d, d), fileSet([]byte(testSVG)))
	require.NoError(t, err)
	assert.Len(t, got.MenuIcons, 1)
}

func TestValidateIcons_SharedFileUsesStricterLimit(t *testing.T) {
	big := svgWithComment(10 * 1024)
	d := IconDigest(big)
	_, err := ValidateIcons(iconDef(d, "", d), fileSet(big))
	require.ErrorContains(t, err, "menu icon")
	require.ErrorContains(t, err, fmt.Sprint(MaxMenuIconBytes))
}

func TestValidateIcons_Rejects(t *testing.T) {
	svgDigest := IconDigest([]byte(testSVG))
	bigLogo := svgWithComment(MaxLogoBytes)
	bigMenu := svgWithComment(MaxMenuIconBytes)
	text := []byte("just some text")
	widePNG := pngOfSize(MaxPNGDimension+1, 1)
	tallPNG := pngOfSize(1, MaxPNGDimension+1)
	tooMany := map[string][]byte{}
	for i := range MaxIconFiles + 1 {
		c := fmt.Appendf(nil, `<svg xmlns="http://www.w3.org/2000/svg"><!--%d--></svg>`, i)
		tooMany[IconDigest(c)] = c
	}

	tests := []struct {
		name  string
		def   *PluginDefinition
		files map[string][]byte
		want  string
	}{
		{"unresolved path", iconDef("icons/a.svg", ""), nil, "icons/a.svg"},
		{"url", iconDef("https://x/y.png", ""), nil, "https://x/y.png"},
		{"data uri", iconDef("", "", "data:image/png;base64,AA"), nil, "data:image/png"},
		{"digest without file", iconDef(svgDigest, ""), nil, "no file"},
		{"digest mismatch", iconDef(svgDigest, ""), map[string][]byte{svgDigest: []byte(strings.ToUpper(testSVG))}, "does not match"},
		{"unreferenced file", iconDef("globe", ""), fileSet([]byte(testSVG)), "not referenced"},
		{"oversized logo", iconDef(IconDigest(bigLogo), ""), fileSet(bigLogo), "logo"},
		{"oversized menu icon", iconDef("", "", IconDigest(bigMenu)), fileSet(bigMenu), "menu icon"},
		{"png menu icon", iconDef("", "", IconDigest(testPNG)), fileSet(testPNG), "menu icon"},
		{"text logo", iconDef(IconDigest(text), ""), fileSet(text), "SVG or PNG"},
		{"iconDark without file icon", iconDef("globe", svgDigest), fileSet([]byte(testSVG)), "iconDark"},
		{"iconDark without icon", iconDef("", svgDigest), fileSet([]byte(testSVG)), "iconDark"},
		{"iconDark nldd name", iconDef(svgDigest, "moon"), fileSet([]byte(testSVG)), "iconDark"},
		{"malformed digest upper", iconDef("", "", "sha256:ABC"), nil, "not a valid digest"},
		{"malformed digest lower", iconDef("sha256:abc", ""), nil, "not a valid digest"},
		{"too many files", iconDef("globe", ""), tooMany, "too many"},
		{"unsupported logo format", iconDef("logo.jpg", ""), nil, `"logo.jpg" is not a built-in icon name`},
		{"unsupported menu format", iconDef("", "", "menu.webp"), nil, `"menu.webp" is not a built-in icon name`},
		{"name with capitals", iconDef("Globe", ""), nil, `"Globe" is not a built-in icon name`},
		{"name with spaces", iconDef("", "", "my icon"), nil, `"my icon" is not a built-in icon name`},
		{"png too wide", iconDef(IconDigest(widePNG), ""), fileSet(widePNG), "1025x1"},
		{"png too tall", iconDef(IconDigest(tallPNG), ""), fileSet(tallPNG), "1x1025"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateIcons(tt.def, tt.files)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

const svgNS = `xmlns="http://www.w3.org/2000/svg"`

func TestValidateIcons_RejectsUnsafeSVG(t *testing.T) {
	tests := []struct{ name, svg, want string }{
		{"not xml", `not xml at all`, "text outside the root"},
		{"truncated", `<svg ` + svgNS + `><path>`, "unexpected EOF"},
		{"root not svg", `<html ` + svgNS + `><body/></html>`, "root element must be"},
		{"two roots", `<svg ` + svgNS + `/><svg ` + svgNS + `/>`, "root element must be"},
		{"text before root", `junk<svg ` + svgNS + `/>`, "text outside the root"},
		{"text after root", `<svg ` + svgNS + `/>junk`, "text outside the root"},
		{"no namespace", `<svg/>`, "SVG namespace"},
		{"xhtml root", `<svg xmlns="http://www.w3.org/1999/xhtml"/>`, "SVG namespace"},
		{"xhtml img", `<svg ` + svgNS + `><img xmlns="http://www.w3.org/1999/xhtml" src="https://evil/x.png"/></svg>`, "SVG namespace"},
		{"xhtml video", `<svg ` + svgNS + ` xmlns:h="http://www.w3.org/1999/xhtml"><h:video src="https://evil/x.mp4"/></svg>`, "SVG namespace"},
		{"xhtml meta refresh", `<svg ` + svgNS + ` xmlns:h="http://www.w3.org/1999/xhtml"><h:meta http-equiv="refresh" content="0;url=https://evil"/></svg>`, "SVG namespace"},
		{"colon element", `<svg ` + svgNS + `><:script/></svg>`, "not allowed"},
		{"colon onload", `<svg ` + svgNS + ` :onload="x()"/>`, "attribute name"},
		{"colon href", `<svg ` + svgNS + `><use :href="https://evil/x"/></svg>`, "attribute name"},
		{"script", `<svg ` + svgNS + `><script>alert(1)</script></svg>`, "<script> is not allowed"},
		{"upper script", `<svg ` + svgNS + `><SCRIPT>alert(1)</SCRIPT></svg>`, "<script> is not allowed"},
		{"prefixed script", `<svg ` + svgNS + ` xmlns:s="http://www.w3.org/2000/svg"><s:script>alert(1)</s:script></svg>`, "<script> is not allowed"},
		{"onload", `<svg ` + svgNS + ` onload="alert(1)"/>`, "event handler"},
		{"upper onclick", `<svg ` + svgNS + `><rect ONCLICK="alert(1)"/></svg>`, "event handler"},
		{"foreignObject", `<svg ` + svgNS + `><foreignObject><div/></foreignObject></svg>`, "<foreignobject> is not allowed"},
		{"href https", `<svg ` + svgNS + `><use href="https://x/y.svg#a"/></svg>`, "local fragment"},
		{"xlink href https", `<svg ` + svgNS + ` xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="https://x/y.svg#a"/></svg>`, "local fragment"},
		{"href javascript", `<svg ` + svgNS + `><a href="javascript:alert(1)"><rect/></a></svg>`, "local fragment"},
		{"href data", `<svg ` + svgNS + `><image href="data:image/png;base64,AA"/></svg>`, "local fragment"},
		{"xml base", `<svg ` + svgNS + ` xml:base="https://evil/"/>`, "xml:base"},
		{"doctype", `<!DOCTYPE svg [<!ENTITY x "y">]><svg ` + svgNS + `/>`, "DOCTYPE"},
		{"stylesheet pi", `<?xml-stylesheet href="x.css"?><svg ` + svgNS + `/>`, "processing instruction"},
		{"style attr url", `<svg ` + svgNS + `><rect style="background:url(https://x/y.png)"/></svg>`, `attribute "style"`},
		{"fill external url", `<svg ` + svgNS + `><rect fill="url(https://x/y.svg#a)"/></svg>`, `attribute "fill"`},
		{"style import", `<svg ` + svgNS + `><style>@import url(https://x/y.css);</style></svg>`, "@import"},
		{"style import bare", `<svg ` + svgNS + `><style>@import "https://x/y.css";</style></svg>`, "@import"},
		{"style element url", `<svg ` + svgNS + `><style>rect{fill:url(https://x/y)}</style></svg>`, "url()"},
		{"style escape", `<svg ` + svgNS + `><style>rect{fill:u\72l(https://x/y)}</style></svg>`, "backslash"},
		{"style split by comment", `<svg ` + svgNS + `><style>@im<!-- x -->port "https://evil/x.css";</style></svg>`, "comments inside <style>"},
		{"style url split by comment", `<svg ` + svgNS + `><style>rect{fill:url<!-- x -->(https://evil/y)}</style></svg>`, "comments inside <style>"},
		{"style split by cdata", `<svg ` + svgNS + `><style><![CDATA[@im]]><![CDATA[port "https://evil/x.css";]]></style></svg>`, "@import"},
		{"style split by element", `<svg ` + svgNS + `><style>@im<a/>port "https://evil/x.css";</style></svg>`, "elements inside <style>"},
		{"css src", `<svg ` + svgNS + `><rect style="fill:src('https://evil/x')"/></svg>`, `"src("`},
		{"css image", `<svg ` + svgNS + `><rect style="fill:image('https://evil/x')"/></svg>`, `"image("`},
		{"css image-set", `<svg ` + svgNS + `><rect style="fill:image-set('https://evil/x' 1x)"/></svg>`, `"image-set("`},
		{"css cross-fade", `<svg ` + svgNS + `><rect style="fill:cross-fade('https://evil/x')"/></svg>`, `"cross-fade("`},
		{"set href", `<svg ` + svgNS + `><a><set attributeName="href" to="javascript:alert(1)"/></a></svg>`, "must not animate"},
		{"animateTransform href", `<svg ` + svgNS + `><a><animateTransform attributeName="href" to="javascript:alert(1)"/></a></svg>`, "must not animate"},
		{"animateMotion onload", `<svg ` + svgNS + `><animateMotion attributeName="onload"/></svg>`, "must not animate"},
		{"comment starting >", `<svg ` + svgNS + `><!--><script>x</script>--></svg>`, "comment starting"},
		{"comment starting ->", `<svg ` + svgNS + `><!---><img/>--></svg>`, "comment starting"},
		{"backslash attribute", `<svg ` + svgNS + ` export-filename="C:\a\b.png"/>`, `attribute "export-filename" contains a backslash`},
		{"png header then svg", "\x89PNG\r\n\x1a\n<svg " + svgNS + ` onload="x()"/>`, "must be an SVG or PNG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.svg)
			_, err := ValidateIcons(iconDef(IconDigest(content), ""), fileSet(content))
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateIcons_AllowsSafeSVG(t *testing.T) {
	tests := []string{
		`<svg ` + svgNS + ` xmlns:xlink="http://www.w3.org/1999/xlink"><defs><linearGradient id="a"/></defs><use href="#a"/><use xlink:href="#a"/><rect fill="url(#a)" style="stroke:url(#a)"/><style>rect{fill:url(#a)}</style></svg>`,
		`<?xml version="1.0" encoding="UTF-8"?><svg ` + svgNS + `/>`,
		`<s:svg xmlns:s="http://www.w3.org/2000/svg"><s:rect/></s:svg>`,
		`<svg ` + svgNS + `><!-- ok --></svg>`,
	}
	for _, svg := range tests {
		content := []byte(svg)
		_, err := ValidateIcons(iconDef(IconDigest(content), ""), fileSet(content))
		require.NoError(t, err, svg)
	}
}

func TestParseDefinition_IgnoresIconRules(t *testing.T) {
	for _, icon := range []string{"https://x/y.png", IconDigest([]byte("x"))} {
		data := fmt.Appendf(nil, `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: p
  icon: %s
  iconDark: %s
spec:
  image: r/i@sha256:%s
  menu:
    organization:
      - crd: x
        icon: %s
`, icon, icon, strings.Repeat("a", 64), icon)
		def, err := ParseDefinition(data)
		require.NoError(t, err)
		assert.Equal(t, icon, def.Metadata.IconDark)
	}
}

func TestIsIconFileRef(t *testing.T) {
	for _, v := range []string{"logo.svg", "LOGO.PNG", "icons/a", "https://x/y"} {
		assert.True(t, IsIconFileRef(v), v)
	}
	for _, v := range []string{"globe", "boxes-3", ""} {
		assert.False(t, IsIconFileRef(v), v)
	}
}

func TestIsIconDigest(t *testing.T) {
	assert.True(t, IsIconDigest(IconDigest([]byte("x"))))
	assert.False(t, IsIconDigest("sha256:ABC"))
	assert.False(t, IsIconDigest("sha256:"+strings.Repeat("a", 64)+"\n"))
}
