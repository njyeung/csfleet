#!/usr/bin/env python3
"""Upload a plugin manifest (.toml) or config file (.cfg) to the csfleet database.

Usage:
    ./upload.py plugin  <file.toml>  [--name NAME]
    ./upload.py config  <file.cfg>   [--name NAME]

Plugin --name defaults to the filename without extension.
Config --name is the game-relative path (e.g. cfg/server.cfg); defaults to
cfg/<filename>.

Upserts: if a row with that name exists, it's replaced.
"""

import argparse
import os
import sys

from dotenv import load_dotenv
import pymysql


def connect():
    env_path = os.path.join(os.path.dirname(__file__), os.pardir, ".env")
    load_dotenv(env_path)

    return pymysql.connect(
        host=os.environ.get("DB_HOST", "127.0.0.1"),
        port=int(os.environ.get("DB_PORT", "3306")),
        user=os.environ.get("DB_USER", "csfleet"),
        password=os.environ.get("DB_PASS", "csfleet"),
        database=os.environ.get("DB_NAME", "csfleet"),
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


def upload_config(conn, name, content):
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO config_files (name, content) VALUES (%s, %s) "
            "ON DUPLICATE KEY UPDATE content = VALUES(content)",
            (name, content),
        )
    print(f"config  {name!r}  ok")


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="kind", required=True)

    pp = sub.add_parser("plugin", help="upload a plugin manifest (.toml)")
    pp.add_argument("file", help="path to the .toml file")
    pp.add_argument("--name", help="override the manifest name (default: filename stem)")

    cp = sub.add_parser("config", help="upload a config file (.cfg)")
    cp.add_argument("file", help="path to the config file")
    cp.add_argument("--name", help="game-relative path used as name (default: cfg/<filename>)")

    args = p.parse_args()
    content = open(args.file).read()
    conn = connect()

    if args.kind == "plugin":
        name = args.name or os.path.splitext(os.path.basename(args.file))[0]
        upload_plugin(conn, name, content)
    else:
        name = args.name or "cfg/" + os.path.basename(args.file)
        upload_config(conn, name, content)


if __name__ == "__main__":
    main()
