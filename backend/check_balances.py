#!/usr/bin/env python3
"""
Check Token Balances for All Rubix Nodes

This script checks the RBT token balance for all running nodes.

Usage:
    python backend/check_balances.py
    python backend/check_balances.py --verbose
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


def get_node_balance(port: int, did: str, verbose: bool = False) -> float:
    """Get balance for a specific node"""
    try:
        url = f"http://localhost:{port}/api/get-account-info?did={did}"
        if verbose:
            print(f"  Querying: {url}")

        response = requests.get(url, timeout=5)

        if response.status_code != 200:
            if verbose:
                print(f"  ❌ HTTP {response.status_code}: {response.text}")
            return -1.0

        data = response.json()

        if not data.get("status", False):
            if verbose:
                print(f"  ❌ API Error: {data.get('message', 'Unknown error')}")
            return -1.0

        account_info = data.get("account_info", [])
        if account_info and len(account_info) > 0:
            return account_info[0].get("rbt_amount", 0.0)

        return 0.0

    except requests.exceptions.RequestException as e:
        if verbose:
            print(f"  ❌ Connection error: {e}")
        return -1.0
    except Exception as e:
        if verbose:
            print(f"  ❌ Error: {e}")
        return -1.0


def check_all_balances(verbose: bool = False):
    """Check balances for all nodes"""
    metadata = load_metadata()

    if not metadata:
        return

    print("\n" + "=" * 80)
    print("RUBIX NODE TOKEN BALANCES")
    print("=" * 80)

    total_balance = 0.0
    quorum_nodes = []
    transaction_nodes = []
    offline_nodes = []

    for node_id, node_data in sorted(metadata.items()):
        port = node_data.get("server_port")
        did = node_data.get("did", "")
        is_quorum = node_data.get("is_quorum", False)

        if verbose:
            print(f"\n[{node_id}]")
            print(f"  Port: {port}")
            print(f"  Type: {'Quorum' if is_quorum else 'Transaction'}")
            print(f"  DID: {did[:20]}..." if len(did) > 20 else f"  DID: {did}")

        balance = get_node_balance(port, did, verbose)

        node_info = {
            "id": node_id,
            "port": port,
            "balance": balance,
            "did": did
        }

        if balance < 0:
            offline_nodes.append(node_info)
        elif is_quorum:
            quorum_nodes.append(node_info)
            total_balance += balance
        else:
            transaction_nodes.append(node_info)
            total_balance += balance

    # Display results
    print("\n" + "-" * 80)
    print("QUORUM NODES:")
    print("-" * 80)
    for node in quorum_nodes:
        print(f"  {node['id']:20s}  Port: {node['port']:5d}  Balance: {node['balance']:8.2f} RBT")

    print("\n" + "-" * 80)
    print("TRANSACTION NODES:")
    print("-" * 80)
    for node in transaction_nodes:
        print(f"  {node['id']:20s}  Port: {node['port']:5d}  Balance: {node['balance']:8.2f} RBT")

    if offline_nodes:
        print("\n" + "-" * 80)
        print("OFFLINE/ERROR NODES:")
        print("-" * 80)
        for node in offline_nodes:
            print(f"  {node['id']:20s}  Port: {node['port']:5d}  ❌ Not responding")

    print("\n" + "=" * 80)
    print(f"SUMMARY:")
    print(f"  Total Nodes:        {len(metadata)}")
    print(f"  Quorum Nodes:       {len(quorum_nodes)}")
    print(f"  Transaction Nodes:  {len(transaction_nodes)}")
    print(f"  Offline Nodes:      {len(offline_nodes)}")
    print(f"  Total Balance:      {total_balance:.2f} RBT")
    print("=" * 80 + "\n")


def main():
    parser = argparse.ArgumentParser(description="Check token balances for all Rubix nodes")
    parser.add_argument("--verbose", "-v", action="store_true", help="Show detailed output")
    args = parser.parse_args()

    check_all_balances(verbose=args.verbose)


if __name__ == "__main__":
    main()
