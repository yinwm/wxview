package sqlitecli

import "net/url"

func ImmutableURI(path string) string {
	u := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "mode=ro&immutable=1",
	}
	return u.String()
}
