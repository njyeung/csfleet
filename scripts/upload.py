#!/usr/bin/env python3
"""Upload a plugin manifest (.toml) or config file (.cfg) to the csfleet database.

Usage:
    ./upload.py plugin  <file.toml>  [--name NAME]
    ./upload.py config  <file.cfg>   --path <game-relative path>  [--name NAME]

The --name defaults to the filename without extension.
Upserts: if a row with that name exists, it's replaced.

Connection defaults match the dev docker-compose; override with env vars:
    DB_HOST  (default: 127.0.0.1)
    DB_PORT  (default: 3306)
    DB_USER  (default: weaponpaints)
    DB_PASS  (default: dbpass)
"""

import argparse
import os
import sys

import pymysql

DB = "csfleet"


def connect():
    return pymysql.connect(
        host=os.environ.get("DB_HOST", "127.0.0.1"),
        port=int(os.environ.get("DB_PORT", "3306")),
        user=os.environ.get("DB_USER", "weaponpaints"),
        password=os.environ.get("DB_PASS", "dbpass"),
        database=DB,
        autocommit=True,
    )


def upload_plugin(conn, name, content):
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO plugin_manifests (name, manifest) VALUES (%s, %s) "
            "ON DUPLICATE KEY UPDATE manifest = VALUES(manifest)",
            (name, content),
        )
    print(f"plugin  {name!r}  ok")


def upload_config(conn, name, content, path):
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO config_files (name, content, path) VALUES (%s, %s, %s) "
            "ON DUPLICATE KEY UPDATE content = VALUES(content), path = VALUES(path)",
            (name, content, path),
        )
    print(f"config  {name!r}  -> {path}  ok")


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="kind", required=True)

    pp = sub.add_parser("plugin", help="upload a plugin manifest (.toml)")
    pp.add_argument("file", help="path to the .toml file")
    pp.add_argument("--name", help="override the manifest name (default: filename stem)")

    cp = sub.add_parser("config", help="upload a config file (.cfg)")
    cp.add_argument("file", help="path to the config file")
    cp.add_argument("--path", required=True, help="game-relative install path (e.g. cfg/server.cfg)")
    cp.add_argument("--name", help="override the config name (default: filename stem)")

    args = p.parse_args()
    name = args.name or os.path.splitext(os.path.basename(args.file))[0]
    content = open(args.file).read()
    conn = connect()

    if args.kind == "plugin":
        upload_plugin(conn, name, content)
    else:
        upload_config(conn, name, content, args.path)


if __name__ == "__main__":
    main()
