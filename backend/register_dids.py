#!/usr/bin/env python3
"""
Re-register DIDs for All Rubix Nodes

This script reads node_metadata.json and re-registers DIDs for all nodes.
Useful when DID registration failed during initial setup.

Usage:
    python backend/register_dids.py
    python backend/register_dids.py --password mypassword
    python backend/register_dids.py --nodes node_20100_1,node_20100_2
"""

import json
import argparse
import requests
from pathlib import Path
from typing import Dict, List


def load_metadata(data_dir: str = "./rubix-data") -> Dict:
    """Load node metadata from file"""
    metadata_file = Path(data_dir) / "node_metadata.json"

    if not metadata_file.exists():
        print(f"❌ Metadata file not found: {metadata_file}")
        print("Please start nodes first using: python backend/rubix_node_manager.py --nodes 20 --fresh")
        return {}

    with open(metadata_file, 'r') as f:
        return json.load(f)


def register_did(port: int, did: str, password: str, node_id: str) -> bool:
    """Register DID for a specific node"""
    base_url = f"http://localhost:{port}"

    try:
        # Step 1: Call register-did endpoint
        print(f"  Registering DID for {node_id} on port {port}...")
        payload = {"did": did}
        response = requests.post(f"{base_url}/api/register-did", json=payload, timeout=30)

        if response.status_code != 200:
            print(f"  ❌ HTTP {response.status_code}: {response.text}")
            return False

        data = response.json()

        # Step 2: Check if password is needed
        if data.get("status", False) and data.get("message") == "Password needed":
            result = data.get("result", {})
            sig_id = result.get("id", "")
            mode = result.get("mode", 0)

            if sig_id:
                print(f"  Password required, sending signature response...")
                return send_signature_response(base_url, sig_id, mode, password)

        # Step 3: Check if already successful
        if data.get("status", False):
            print(f"  ✓ DID registered successfully")
            return True

        print(f"  ❌ Registration failed: {data.get('message', 'Unknown error')}")
        return False

    except requests.exceptions.RequestException as e:
        print(f"  ❌ Connection error: {e}")
        return False
    except Exception as e:
        print(f"  ❌ Error: {e}")
        return False


def send_signature_response(base_url: str, sig_id: str, mode: int, password: str) -> bool:
    """Send signature response with password"""
    try:
        payload = {
            "id": sig_id,
            "mode": mode,
            "password": password
        }

        response = requests.post(f"{base_url}/api/signature-response", json=payload, timeout=30)

        if response.status_code != 200:
            print(f"  ❌ Signature response failed: HTTP {response.status_code}")
            return False

        data = response.json()
        if data.get("status", False):
            print(f"  ✓ Signature response accepted")
            return True

        print(f"  ❌ Signature response failed: {data.get('message', 'Unknown error')}")
        return False

    except Exception as e:
        print(f"  ❌ Failed to send signature response: {e}")
        return False


def register_all_dids(password: str, specific_nodes: List[str] = None):
    """Register DIDs for all nodes (or specific nodes)"""
    metadata = load_metadata()

    if not metadata:
        return

    print("\n" + "=" * 80)
    print("RUBIX DID RE-REGISTRATION")
    print("=" * 80)

    # Filter nodes if specific ones requested
    if specific_nodes:
        nodes_to_process = {k: v for k, v in metadata.items() if k in specific_nodes}
        if not nodes_to_process:
            print(f"❌ None of the specified nodes found in metadata")
            return
        print(f"Processing {len(nodes_to_process)} specific node(s)")
    else:
        nodes_to_process = metadata
        print(f"Processing all {len(nodes_to_process)} nodes from metadata")

    successful = []
    failed = []
    skipped = []

    for node_id, node_data in sorted(nodes_to_process.items()):
        port = node_data.get("server_port")
        did = node_data.get("did", "")
        is_quorum = node_data.get("is_quorum", False)

        print(f"\n[{node_id}]")
        print(f"  Port: {port}")
        print(f"  Type: {'Quorum' if is_quorum else 'Transaction'}")
        print(f"  DID: {did[:20]}..." if len(did) > 20 else f"  DID: {did}")

        if not did:
            print(f"  ⚠ No DID found in metadata, skipping...")
            skipped.append(node_id)
            continue

        # Check if node is running
        try:
            health_response = requests.get(f"http://localhost:{port}/api/node-status", timeout=5)
            if health_response.status_code != 200:
                print(f"  ⚠ Node not responding on port {port}, skipping...")
                skipped.append(node_id)
                continue
        except:
            print(f"  ⚠ Node not running on port {port}, skipping...")
            skipped.append(node_id)
            continue

        # Register DID
        if register_did(port, did, password, node_id):
            successful.append(node_id)
        else:
            failed.append(node_id)

    # Summary
    print("\n" + "=" * 80)
    print("SUMMARY:")
    print("=" * 80)
    print(f"  Total nodes processed: {len(nodes_to_process)}")
    print(f"  Successfully registered: {len(successful)}")
    print(f"  Failed: {len(failed)}")
    print(f"  Skipped (offline/no DID): {len(skipped)}")

    if successful:
        print(f"\n✓ Successfully registered:")
        for node_id in successful:
            print(f"    - {node_id}")

    if failed:
        print(f"\n✗ Failed to register:")
        for node_id in failed:
            print(f"    - {node_id}")

    if skipped:
        print(f"\n⚠ Skipped:")
        for node_id in skipped:
            print(f"    - {node_id}")

    print("=" * 80 + "\n")


def main():
    parser = argparse.ArgumentParser(description="Re-register DIDs for Rubix nodes from metadata")
    parser.add_argument("--password", "-p", default="mypassword",
                       help="Password for DID operations (default: mypassword)")
    parser.add_argument("--nodes", "-n",
                       help="Comma-separated list of specific nodes to process (e.g., node_20100_1,node_20100_2)")

    args = parser.parse_args()

    specific_nodes = None
    if args.nodes:
        specific_nodes = [n.strip() for n in args.nodes.split(',')]
        print(f"Will process specific nodes: {specific_nodes}")

    register_all_dids(args.password, specific_nodes)


if __name__ == "__main__":
    main()
