// httptranslate is a helper library to increase code reusability in other projects
// It provides simple but effective way to obtain, store and use language information provided by the user.
// It is a wrapper around excellent golang.org/x/text/language
// This library forces you to provide every translation in all specified languages.
package httptranslate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"reflect"

	"golang.org/x/text/language"
)

type langContextKey int

// Generic http error - useful when there is error with translations and we do not know user language yet.
const HttpGenericError string = "Internal server error - please try again in a few minutes."

const ctxLang langContextKey = 0

var langTags []language.Tag

var matcher language.Matcher = language.NewMatcher(langTags)

// Accept languages used in your application as language tags.
// You need to use it before any other function from this package is used.
// First language provided will be used as fallback if package cannot determine user language.
func SetLangTags(l ...language.Tag) {
	langTags = l
}

// Accepts map that will contain translation structs for every language tag.
// Additionally you have to provide empty translation struct to use it as a base for other languages
// Function reads from lang/langTag.json, where langTag is string representation of language tag.
// If the json is missing, malformed or does not match your translation struct one-to-one an non-nil error is returned.
func LoadTexts[T any](l map[language.Tag]*T, t T) error {
	if len(langTags) == 0 {
		return errors.New("You need to use SetLangTags before LoadTexts")
	}
	for _, lang := range langTags {
		t_copy := t
		l[lang] = &t_copy
		if err := load(l[lang], "lang/"+lang.String()+".json"); err != nil {
			return err
		}
	}
	return nil
}

// Http middleware that provides language as a context value for later http.Handler.
// You can get this value by calling GetLang() later.
// It determines user language in the following order:
// - cookie named lang exists, its value is parsed and used (parsing uses "golang.org/x/text/language" Matcher)
// - http header Accept-Language, its value is parsed and used (as above). Additionaly, a cookie named lang is set.
// - uses fallback language (derived from SetLangTags())
func LangMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tag language.Tag
		lang, err := r.Cookie("lang")
		switch err {
		case nil:
			tag = getSimpleTag(lang.Value)
		case http.ErrNoCookie:
			tag = getSimpleTag(r.Header.Get("Accept-Language"))
			cookie := http.Cookie{
				Name:     "lang",
				Value:    tag.String(),
				Path:     "/",
				MaxAge:   60 * 60 * 24 * 365,
				HttpOnly: false,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			}
			http.SetCookie(w, &cookie)
		default:
			// empty getSimpleTag uses default language
			tag = getSimpleTag("")
			slog.Error("Unknown error when reading lang cookie",
				slog.Any("err", err),
			)
		}
		r = httpContextSetLang(r, tag)
		next.ServeHTTP(w, r)
	})
}

// Returns language tag from context value (provided by LangMiddleware()).
// If the value is missing returns non nil error.
func GetLang(r *http.Request) (language.Tag, error) {
	val, ok := r.Context().Value(ctxLang).(language.Tag)
	if !ok {
		return val, errors.New("Could not get Lang form http context")
	}
	return val, nil
}

func httpContextSetLang(r *http.Request, val language.Tag) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxLang, val))
}

func noneFieldEmpty(s any) error {
	v := reflect.ValueOf(s)
	typeOfS := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if typeOfS.Field(i).Name == "string" && v.Field(i).String() == "" {
		}
		switch typeOfS.Field(i).Type.Kind() {
		case reflect.String:
			if v.Field(i).String() == "" {
				return fmt.Errorf("Field: %s\t is empty!\n", typeOfS.Field(i).Name)
			}
		case reflect.Struct:
			if err := noneFieldEmpty(v.Field(i).Interface()); err != nil {
				return fmt.Errorf("Field: %s: %w", typeOfS.Field(i).Name, err)
			}
		default:
			return fmt.Errorf("Unknown kind\tField: %s\tType: %s\tValue: %v\n", typeOfS.Field(i).Name, typeOfS.Field(i).Type.Kind(), v.Field(i).Interface())
		}
	}
	return nil
}

func load[T any](t *T, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(t); err != nil {
		return err
	}
	return noneFieldEmpty(*t)
}

func getSimpleTag(s string) language.Tag {
	_, i := language.MatchStrings(matcher, s)
	return langTags[i]
}
