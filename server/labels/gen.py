#!/usr/bin/env python3
"""Builds server/labels/presets.json: the names of the iD presets (feature types) in English, German,
French and Italian, by preset id ("amenity/fast_food": ["Fast Food", "Schnellimbiss", "Restauration
rapide", "Fast food"]). overpass-search names what it found in English, with these ids and English
names (its word list is built from the same version); server/labels.go puts them in the language of
the page. Run it by hand when overpass-search takes another version:

    python3 -I server/labels/gen.py server/labels/presets.json

Source: @openstreetmap/id-tagging-schema (ISC licence), from cdn.jsdelivr.net.
"""
import json
import sys
import urllib.request

ID_VERSION = "7.2.0"  # as overpass-search (tools/gen_search_vocab.py there)
LANGS = ["en", "de", "fr", "it"]


def names(lang):
    url = f"https://cdn.jsdelivr.net/npm/@openstreetmap/id-tagging-schema@{ID_VERSION}/dist/translations/{lang}.min.json"
    with urllib.request.urlopen(url, timeout=120) as r:
        return {k: v.get("name", "") for k, v in json.load(r)[lang]["presets"]["presets"].items()}


by = {lang: names(lang) for lang in LANGS}
out = {}
for pid, en in sorted(by["en"].items()):
    if not en:
        continue
    en = en[0].upper() + en[1:]  # as overpass-search writes it (vocab.Capitalize)
    row = [en] + [by[lang].get(pid, "") for lang in LANGS[1:]]
    row[1] = row[1].replace("ß", "ss")  # Swiss spelling, as the texts of the page
    if row[1] in ("", en) and en.endswith(" Fast Food"):  # not translated in iD: "Sushi Fast Food" is a Sushi-Imbiss
        row[1] = en[: -len(" Fast Food")] + "-Imbiss"
    out[pid] = [row[0]] + [n if n != en else "" for n in row[1:]]  # "" where the name is the English one
with open(sys.argv[1], "w", encoding="utf-8") as f:
    json.dump(out, f, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    f.write("\n")
print(f"{len(out)} presets", file=sys.stderr)
