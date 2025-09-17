#!/usr/bin/env python3
"""
Rubix Node Manager - Python Implementation

This script replicates the exact node startup and quorum setup process 
from the Go implementation in backend/internal/rubix/manager.go

Usage:
    python rubix_node_manager.py --nodes 10 --fresh
    python rubix_node_manager.py --nodes 5
    python rubix_node_manager.py --restart
"""

import os
import sys
import json
import time
import shutil
import argparse
import platform
import subprocess
import requests
from pathlib import Path
from typing import Dict, List, Optional, Tuple
import logging

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('rubix_manager.log'),
        logging.StreamHandler(sys.stdout)
    ]
)
logger = logging.getLogger(__name__)

class RubixConfig:
    """Configuration class matching Go config structure"""
    
    def __init__(self):
        self.data_dir = "./rubix-data"
        self.base_server_port = 20000
        self.base_grpc_port = 10500
        self.quorum_node_count = 7
        self.min_transaction_nodes = 2
        self.max_transaction_nodes = 20
        self.node_startup_timeout = 120  # seconds
        self.default_priv_key_password = "mypassword"
        self.default_quorum_key_password = "mypassword"
        self.rubix_repo_url = "https://github.com/rubixchain/rubixgoplatform.git"
        self.rubix_branch = "main"
        self.ipfs_version = "v0.21.0"
        self.test_swarm_key_url = "https://raw.githubusercontent.com/rubixchain/rubixgoplatform/main/testswarm.key"

class NodeInfo:
    """Node information class matching Go NodeInfo struct"""
    
    def __init__(self, node_id: str, server_port: int, grpc_port: int, 
                 did: str = "", peer_id: str = "", is_quorum: bool = False, 
                 status: str = "stopped"):
        self.id = node_id
        self.server_port = server_port
        self.grpc_port = grpc_port
        self.did = did
        self.peer_id = peer_id
        self.is_quorum = is_quorum
        self.status = status
        self.process = None

    def to_dict(self):
        """Convert to dictionary for JSON serialization"""
        return {
            "id": self.id,
            "server_port": self.server_port,
            "grpc_port": self.grpc_port,
            "did": self.did,
            "peer_id": self.peer_id,
            "is_quorum": self.is_quorum,
            "status": self.status
        }

    @classmethod
    def from_dict(cls, data: dict):
        """Create NodeInfo from dictionary"""
        return cls(
            node_id=data["id"],
            server_port=data["server_port"],
            grpc_port=data["grpc_port"],
            did=data.get("did", ""),
            peer_id=data.get("peer_id", ""),
            is_quorum=data.get("is_quorum", False),
            status=data.get("status", "stopped")
        )

class RubixClient:
    """HTTP client for communicating with Rubix nodes"""
    
    def __init__(self, base_url: str):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.timeout = 30

    def wait_for_node(self, timeout: int = 120) -> bool:
        """Wait for node to be ready"""
        logger.info(f"  Waiting for node at {self.base_url} to be ready (timeout: {timeout}s)...")
        
        start_time = time.time()
        while time.time() - start_time < timeout:
            try:
                response = self.session.get(f"{self.base_url}/api/basic-info")
                if response.status_code == 200:
                    logger.info(f"  ✓ Node at {self.base_url} is ready")
                    return True
            except requests.exceptions.RequestException:
                pass
            
            time.sleep(2)
        
        logger.error(f"  ✗ Node at {self.base_url} failed to start within {timeout}s")
        return False

    def create_did(self, password: str) -> Tuple[str, str]:
        """Create DID for the node"""
        logger.info(f"  Creating DID for node at {self.base_url}...")
        
        payload = {"privPWD": password}
        response = self.session.post(f"{self.base_url}/api/create-did", json=payload)
        
        if response.status_code != 200:
            raise Exception(f"Failed to create DID: {response.status_code} - {response.text}")
        
        data = response.json()
        if not data.get("status", False):
            raise Exception(f"Create DID failed: {data.get('message', 'Unknown error')}")
        
        result = data.get("result", {})
        did = result.get("did", "")
        peer_id = result.get("peerID", "")
        
        # Log with truncated values for readability
        did_display = did[:16] + "..." if len(did) > 16 else did
        peer_id_display = peer_id[:8] + "..." if len(peer_id) > 8 else peer_id
        
        if not peer_id:
            logger.warning(f"  ⚠ DID created: {did_display} (WARNING: PeerID is empty!)")
        else:
            logger.info(f"  ✓ DID created: {did_display} (PeerID: {peer_id_display})")
        
        return did, peer_id

    def register_did(self, did: str, password: str) -> bool:
        """Register DID with the network"""
        logger.info(f"  Registering DID for node at {self.base_url}...")
        
        payload = {"did": did}
        response = self.session.post(f"{self.base_url}/api/register-did", json=payload)
        
        if response.status_code != 200:
            logger.error(f"  ✗ Failed to register DID: {response.status_code} - {response.text}")
            return False
        
        data = response.json()
        if data.get("status", False) and data.get("message") == "Password needed":
            # Handle signature response
            result = data.get("result", {})
            sig_id = result.get("id", "")
            mode = result.get("mode", 0)
            
            if sig_id:
                return self._send_signature_response(sig_id, mode, password)
        
        if not data.get("status", False):
            logger.error(f"  ✗ Register DID failed: {data.get('message', 'Unknown error')}")
            return False
        
        logger.info(f"  ✓ DID registered successfully")
        return True

    def add_quorum(self, quorum_list: List[dict]) -> bool:
        """Add quorum list to the node"""
        logger.info(f"  Adding quorum list to node at {self.base_url}...")
        
        response = self.session.post(f"{self.base_url}/api/add-quorum", json=quorum_list)
        
        if response.status_code != 200:
            logger.error(f"  ✗ Failed to add quorum: {response.status_code} - {response.text}")
            return False
        
        data = response.json()
        if not data.get("status", False):
            logger.error(f"  ✗ Add quorum failed: {data.get('message', 'Unknown error')}")
            return False
        
        logger.info(f"  ✓ Successfully added quorum list")
        return True

    def setup_quorum(self, did: str, password: str, priv_password: str) -> bool:
        """Setup node as quorum member"""
        logger.info(f"  Setting up quorum for node at {self.base_url}...")
        
        payload = {
            "did": did,
            "password": password,
            "priv_password": priv_password
        }
        
        response = self.session.post(f"{self.base_url}/api/setup-quorum", json=payload)
        
        if response.status_code != 200:
            logger.error(f"  ✗ Failed to setup quorum: {response.status_code} - {response.text}")
            return False
        
        data = response.json()
        if not data.get("status", False):
            logger.error(f"  ✗ Setup quorum failed: {data.get('message', 'Unknown error')}")
            return False
        
        logger.info(f"  ✓ Successfully setup quorum")
        return True

    def generate_test_tokens(self, did: str, num_tokens: int, password: str) -> bool:
        """Generate test tokens for the node"""
        logger.info(f"  Generating {num_tokens} test tokens for node at {self.base_url}...")
        
        payload = {
            "number_of_tokens": num_tokens,
            "did": did
        }
        
        response = self.session.post(f"{self.base_url}/api/generate-test-token", json=payload)
        
        if response.status_code != 200:
            logger.error(f"  ✗ Failed to generate tokens: {response.status_code} - {response.text}")
            return False
        
        data = response.json()
        if data.get("status", False) and data.get("message") == "Password needed":
            # Handle signature response
            result = data.get("result", {})
            sig_id = result.get("id", "")
            mode = result.get("mode", 0)
            
            if sig_id:
                success = self._send_signature_response(sig_id, mode, password)
                if success:
                    # Wait and check balance
                    return self._verify_token_generation(did)
        
        return False

    def get_account_balance(self, did: str) -> float:
        """Get account balance for a DID"""
        response = self.session.get(f"{self.base_url}/api/get-account-info", params={"did": did})
        
        if response.status_code != 200:
            raise Exception(f"Failed to get account info: {response.status_code}")
        
        data = response.json()
        if not data.get("status", False):
            raise Exception(f"Get account info failed: {data.get('message', 'Unknown error')}")
        
        account_info = data.get("account_info", [])
        if account_info:
            return account_info[0].get("rbt_amount", 0.0)
        
        return 0.0

    def _send_signature_response(self, sig_id: str, mode: int, password: str) -> bool:
        """Send signature response with password"""
        logger.info(f"  Password required, sending signature response...")
        
        payload = {
            "id": sig_id,
            "mode": mode,
            "password": password
        }
        
        response = self.session.post(f"{self.base_url}/api/signature-response", json=payload)
        
        if response.status_code != 200:
            logger.error(f"  ✗ Failed to send signature response: {response.status_code}")
            return False
        
        data = response.json()
        if not data.get("status", False):
            logger.error(f"  ✗ Signature response failed: {data.get('message', 'Unknown error')}")
            return False
        
        logger.info(f"  ✓ Signature response sent successfully")
        return True

    def _verify_token_generation(self, did: str, max_attempts: int = 10) -> bool:
        """Verify tokens were generated by checking balance"""
        logger.info(f"  Waiting for async token generation...")
        
        for i in range(max_attempts):
            time.sleep(5)
            try:
                balance = self.get_account_balance(did)
                logger.info(f"  Check {i+1}: Current balance: {balance:.2f} RBT")
                if balance > 0:
                    logger.info(f"  ✓ Tokens generated! Final balance: {balance:.2f} RBT")
                    return True
            except Exception as e:
                logger.warning(f"  Check {i+1}: Failed to get balance: {e}")
        
        logger.warning(f"  ⚠ Token generation may have failed - balance still 0 after {max_attempts * 5}s")
        return False

class RubixManager:
    """Main manager class for Rubix nodes"""
    
    def __init__(self, config: RubixConfig):
        self.config = config
        self.nodes: Dict[str, NodeInfo] = {}
        self.data_dir = Path(config.data_dir)
        self.metadata_file = self.data_dir / "node_metadata.json"
        self.rubix_path = self.data_dir / "rubixgoplatform"
        
        # Create data directory
        self.data_dir.mkdir(exist_ok=True)

    def start_nodes(self, transaction_node_count: int, fresh: bool = False) -> bool:
        """Start nodes with the specified configuration"""
        
        if transaction_node_count < self.config.min_transaction_nodes:
            raise ValueError(f"Minimum {self.config.min_transaction_nodes} transaction nodes required")
        
        if transaction_node_count > self.config.max_transaction_nodes:
            raise ValueError(f"Maximum {self.config.max_transaction_nodes} transaction nodes allowed")
        
        # Check for existing metadata
        if not fresh and self.metadata_file.exists():
            logger.info("Found existing node setup. Selecting active nodes...")
            return self._adjust_node_count(transaction_node_count)
        
        # Fresh start: start all 20 nodes
        logger.info("Fresh start: starting all 20 transaction nodes...")
        transaction_node_count = self.config.max_transaction_nodes
        
        # Clean up if fresh start requested
        if fresh:
            logger.info("Fresh start requested, cleaning up existing data...")
            self._cleanup()
        
        # Setup platform
        if not self._setup_rubix_platform():
            return False
        
        total_nodes = self.config.quorum_node_count + transaction_node_count
        
        # Phase 1: Start all nodes
        logger.info("\n================== PHASE 1: Starting Nodes ==================")
        logger.info(f"Total nodes to start: {total_nodes} (Quorum: {self.config.quorum_node_count}, Transaction: {total_nodes - self.config.quorum_node_count})")
        
        quorum_list = []
        
        for i in range(total_nodes):
            node_id = f"node{i}"
            server_port = self.config.base_server_port + i
            grpc_port = self.config.base_grpc_port + i
            is_quorum = i < self.config.quorum_node_count
            
            node_type = "quorum" if is_quorum else "transaction"
            logger.info(f"[{i+1}/{total_nodes}] Starting {node_id} ({node_type} node) on port {server_port}")
            
            # Start node process
            if not self._start_node_process(node_id, i):
                return False
            
            # Wait for node to be ready
            client = RubixClient(f"http://localhost:{server_port}")
            if not client.wait_for_node(self.config.node_startup_timeout):
                return False
            
            # Create DID
            logger.info(f"  Creating DID for {node_id} with password...")
            try:
                did, peer_id = client.create_did(self.config.default_priv_key_password)
            except Exception as e:
                logger.error(f"Failed to create DID for {node_id}: {e}")
                return False
            
            # Store node info
            node_info = NodeInfo(
                node_id=node_id,
                server_port=server_port,
                grpc_port=grpc_port,
                did=did,
                peer_id=peer_id,
                is_quorum=is_quorum,
                status="running"
            )
            
            self.nodes[node_id] = node_info
            
            if is_quorum:
                # Add to quorum list
                quorum_list.append({
                    "type": 2,
                    "address": did
                })
                logger.info(f"  Added {node_id} to quorum list (total quorum members: {len(quorum_list)})")
        
        # Phase 2: DID Registration
        logger.info("\n================== PHASE 2: DID Registration ==================")
        logger.info(f"Registering all {len(self.nodes)} DIDs with the network...")
        
        registration_success = 0
        for node_id, node_info in self.nodes.items():
            node_type = "quorum" if node_info.is_quorum else "transaction"
            logger.info(f"[{node_id}] Registering DID for {node_type} node...")
            
            client = RubixClient(f"http://localhost:{node_info.server_port}")
            if client.register_did(node_info.did, self.config.default_priv_key_password):
                registration_success += 1
            else:
                logger.warning(f"  ⚠ WARNING: Failed to register DID for {node_id}")
        
        logger.info(f"DID registration complete: {registration_success}/{len(self.nodes)} nodes registered")
        
        # Phase 3: Quorum List Distribution
        logger.info("\n================== PHASE 3: Quorum List Distribution ==================")
        logger.info(f"Distributing quorum list to all {len(self.nodes)} nodes...")
        
        quorum_add_success = 0
        for node_id, node_info in self.nodes.items():
            node_type = "quorum" if node_info.is_quorum else "transaction"
            logger.info(f"[{node_id}] Adding quorum list to {node_type} node...")
            
            client = RubixClient(f"http://localhost:{node_info.server_port}")
            if client.add_quorum(quorum_list):
                quorum_add_success += 1
            else:
                logger.error(f"  ✗ ERROR: Failed to add quorum to {node_id}")
        
        logger.info(f"Quorum configuration complete: {quorum_add_success}/{len(self.nodes)} nodes configured")
        
        # Phase 4: Quorum Setup
        logger.info("\n================== PHASE 4: Quorum Setup ==================")
        logger.info(f"Setting up {self.config.quorum_node_count} quorum nodes with quorum-specific configuration...")
        
        quorum_setup_success = 0
        for node_id, node_info in self.nodes.items():
            if node_info.is_quorum:
                client = RubixClient(f"http://localhost:{node_info.server_port}")
                logger.info(f"[{node_id}] Setting up quorum configuration...")
                
                if client.setup_quorum(
                    node_info.did,
                    self.config.default_quorum_key_password,
                    self.config.default_priv_key_password
                ):
                    quorum_setup_success += 1
                else:
                    logger.warning(f"  ⚠ WARNING: Failed to setup quorum for {node_id}")
        
        logger.info(f"Quorum setup complete: {quorum_setup_success}/{self.config.quorum_node_count} quorum nodes configured")
        
        # Phase 5: Token Generation
        logger.info("\n================== PHASE 5: Token Generation ==================")
        logger.info(f"Generating 100 test RBT tokens for all {len(self.nodes)} nodes...")
        
        token_gen_success = 0
        for node_id, node_info in self.nodes.items():
            node_type = "quorum" if node_info.is_quorum else "transaction"
            did_display = node_info.did[:16] + "..." if len(node_info.did) > 16 else node_info.did
            logger.info(f"[{node_id}] Generating test tokens for {node_type} node (DID: {did_display})...")
            
            client = RubixClient(f"http://localhost:{node_info.server_port}")
            
            # Try token generation with retries
            max_retries = 2
            token_generated = False
            
            for attempt in range(1, max_retries + 1):
                if attempt > 1:
                    logger.info(f"  Retry {attempt}/{max_retries} for {node_id}...")
                
                if client.generate_test_tokens(node_info.did, 100, self.config.default_priv_key_password):
                    # Verify tokens were generated
                    logger.info(f"  Checking balance for {node_id}...")
                    try:
                        balance = client.get_account_balance(node_info.did)
                        logger.info(f"  Balance for {node_id}: {balance:.3f} RBT")
                        
                        if balance > 0:
                            logger.info(f"  ✓ Successfully generated tokens for {node_id} (Balance: {balance:.3f} RBT)")
                            token_generated = True
                            token_gen_success += 1
                            break
                        elif attempt < max_retries:
                            logger.warning(f"  ⚠ Balance is 0, retrying token generation...")
                            time.sleep(5)
                        else:
                            logger.error(f"  ✗ ERROR: {node_id} still has 0 balance after {max_retries} attempts!")
                    except Exception as e:
                        logger.error(f"  ✗ Failed to check balance: {e}")
                        break
                else:
                    logger.error(f"  ✗ Failed to generate tokens (attempt {attempt})")
                    if attempt == max_retries:
                        break
            
            if not token_generated:
                logger.error(f"  ✗ FAILED: Token generation failed for {node_id}")
        
        logger.info(f"Token generation complete: {token_gen_success}/{len(self.nodes)} nodes have tokens")
        
        # Phase 6: Finalization
        logger.info("\n================== PHASE 6: Finalization ==================")
        if not self._save_metadata():
            logger.warning("⚠ Warning: failed to save metadata")
        else:
            logger.info("✓ Metadata saved successfully")
        
        # Summary
        logger.info("\n================== SETUP COMPLETE ==================")
        logger.info("Summary:")
        logger.info(f"  - Nodes started: {len(self.nodes)}/{total_nodes}")
        logger.info(f"  - DIDs registered: {registration_success}/{len(self.nodes)}")
        logger.info(f"  - Quorum configured: {quorum_add_success}/{len(self.nodes)}")
        logger.info(f"  - Quorum setup: {quorum_setup_success}/{self.config.quorum_node_count}")
        logger.info(f"  - Tokens generated: {token_gen_success}/{len(self.nodes)}")
        
        if (registration_success < len(self.nodes) or 
            quorum_add_success < len(self.nodes) or 
            token_gen_success < len(self.nodes)):
            logger.warning("⚠ WARNING: Some operations failed. Check logs above for details.")
        else:
            logger.info("✓ All nodes successfully configured and ready!")
        
        return True

    def _start_node_process(self, node_id: str, index: int) -> bool:
        """Start a single node process"""
        
        # Calculate ports
        port = self.config.base_server_port + index
        grpc_port = self.config.base_grpc_port + index
        
        # Create node directory
        node_dir = self.data_dir / "nodes" / node_id
        node_dir.mkdir(parents=True, exist_ok=True)
        
        # Get absolute paths and OS-specific build directory (matching Go implementation)
        abs_data_dir = os.path.abspath(str(self.data_dir))
        abs_rubix_path = os.path.join(abs_data_dir, "rubixgoplatform")
        build_dir_name = self._get_build_dir()
        
        # Platform-specific binary names
        if platform.system() == "Windows":
            rubix_bin = "rubixgoplatform.exe"
            ipfs_bin = "ipfs.exe"
        else:
            rubix_bin = "rubixgoplatform"
            ipfs_bin = "ipfs"
        
        # Source paths in OS-specific build directory (matching Go)
        src_rubix = os.path.join(abs_rubix_path, build_dir_name, rubix_bin)
        src_ipfs = os.path.join(abs_rubix_path, build_dir_name, ipfs_bin)
        src_swarm_key = os.path.join(abs_rubix_path, build_dir_name, "testswarm.key")
        
        # Destination paths (convert node_dir to string for os.path.join)
        node_dir_str = str(node_dir)
        dest_rubix = os.path.join(node_dir_str, rubix_bin)
        dest_ipfs = os.path.join(node_dir_str, ipfs_bin)
        dest_swarm_key = os.path.join(node_dir_str, "testswarm.key")
        
        # Copy files if they don't exist (matching Go logic)
        if not os.path.exists(dest_rubix):
            logger.info(f"Copying rubixgoplatform to {node_dir}")
            shutil.copy2(src_rubix, dest_rubix)
            if platform.system() != "Windows":
                os.chmod(dest_rubix, 0o755)
        
        if not os.path.exists(dest_ipfs):
            logger.info(f"Copying IPFS binary to {node_dir}")
            shutil.copy2(src_ipfs, dest_ipfs)
            if platform.system() != "Windows":
                os.chmod(dest_ipfs, 0o755)
        
        if not os.path.exists(dest_swarm_key):
            logger.info(f"Copying testswarm.key to {node_dir}")
            shutil.copy2(src_swarm_key, dest_swarm_key)
        
        # Build command arguments
        args = [
            "run",
            "-p", node_id,
            "-n", str(index),
            "-s",
            "-port", str(port),
            "-testNet",
            "-grpcPort", str(grpc_port)
        ]
        
        # Create platform-specific command
        if platform.system() == "Windows":
            # Create batch file
            window_title = f"Rubix Node {node_id} - Port {port}"
            
            batch_content = f"""@echo off
title {window_title}
echo Starting {node_id} on port {port}...
echo Node directory: {node_dir}
echo.
cd /d "{node_dir}"
if not exist "{rubix_bin}" (
    echo ERROR: {rubix_bin} not found in node directory
    echo Please ensure all files are copied correctly.
    pause > nul
    exit /b 1
)
if not exist "{ipfs_bin}" (
    echo ERROR: {ipfs_bin} not found in node directory
    echo Please ensure IPFS is copied correctly.
    pause > nul
    exit /b 1
)
if not exist "testswarm.key" (
    echo ERROR: testswarm.key not found in node directory
    echo Please ensure swarm key is copied correctly.
    pause > nul
    exit /b 1
)
"{rubix_bin}" {' '.join(args)}
echo.
echo Node stopped. Press any key to close this window...
pause > nul"""
            
            # Write batch file
            batch_path = self.data_dir / f"node_{node_id}.bat"
            batch_path.write_text(batch_content)
            
            # Start batch file in new window
            cmd = ["cmd", "/c", "start", "", str(batch_path)]
            
        else:
            # Linux/Mac: use tmux session
            session_name = f"rubix-node-{node_id}"
            node_command = f"cd {node_dir} && ./{rubix_bin} {' '.join(args)}"
            cmd = ["tmux", "new-session", "-d", "-s", session_name, node_command]
        
        # Environment variables
        env = os.environ.copy()
        env.update({
            "RUBIX_NODE_DIR": str(node_dir),
            "RUBIX_NODE_ID": node_id
        })
        
        # Log command details
        logger.info(f"Starting node {node_id} from directory: {node_dir}")
        logger.info(f"Command: {rubix_bin} {' '.join(args)}")
        
        # Start process
        try:
            process = subprocess.Popen(cmd, env=env)
            logger.info(f"Node {node_id} process started successfully")
            
            # Store process reference if we have node info
            if node_id in self.nodes:
                self.nodes[node_id].process = process
            
            # Give node time to boot
            time.sleep(30)
            return True
            
        except Exception as e:
            logger.error(f"Failed to start node process: {e}")
            return False

    def _get_build_dir(self) -> str:
        """Get build directory based on OS (matching Go implementation)"""
        system = platform.system().lower()
        if system == "windows":
            return "windows"
        elif system == "linux":
            return "linux"
        elif system == "darwin":
            return "mac"
        else:
            return "build"

    def _setup_rubix_platform(self) -> bool:
        """Setup rubixgoplatform if needed"""
        logger.info("Setting up Rubix platform...")
        
        # For simplicity, assume platform is already built
        # In production, you would implement platform download/build logic here
        build_dir_name = self._get_build_dir()
        build_dir = self.rubix_path / build_dir_name
        
        if platform.system() == "Windows":
            rubix_bin = "rubixgoplatform.exe"
            ipfs_bin = "ipfs.exe"
        else:
            rubix_bin = "rubixgoplatform"
            ipfs_bin = "ipfs"
        
        required_files = [
            build_dir / rubix_bin,
            build_dir / ipfs_bin,
            build_dir / "testswarm.key"
        ]
        
        logger.info(f"Checking binaries in: {build_dir}")
        for file_path in required_files:
            if not file_path.exists():
                logger.error(f"Required file not found: {file_path}")
                logger.error("Please ensure Rubix platform is built and binaries are available")
                return False
        
        logger.info("✓ Rubix platform setup verified")
        return True

    def _adjust_node_count(self, requested_transaction_nodes: int) -> bool:
        """Adjust active node count from existing metadata"""
        
        metadata = self._load_metadata()
        if not metadata:
            logger.error("Failed to load metadata")
            return False
        
        logger.info(f"Adjusting active nodes: selecting {requested_transaction_nodes} transaction nodes from a total of 20")
        
        # Reset current nodes
        self.nodes = {}
        
        # Always include quorum nodes
        for node_id, node_data in metadata.items():
            node_info = NodeInfo.from_dict(node_data)
            if node_info.is_quorum:
                self.nodes[node_id] = node_info
        
        # Select first N transaction nodes
        transaction_nodes_added = 0
        for i in range(self.config.max_transaction_nodes):
            node_id = f"node{self.config.quorum_node_count + i}"
            if node_id in metadata and transaction_nodes_added < requested_transaction_nodes:
                node_data = metadata[node_id]
                node_info = NodeInfo.from_dict(node_data)
                if not node_info.is_quorum:
                    self.nodes[node_id] = node_info
                    transaction_nodes_added += 1
        
        logger.info(f"Selected {self.config.quorum_node_count} quorum nodes and {transaction_nodes_added} transaction nodes")
        return True

    def _save_metadata(self) -> bool:
        """Save node metadata to file"""
        try:
            metadata = {node_id: node_info.to_dict() for node_id, node_info in self.nodes.items()}
            with open(self.metadata_file, 'w') as f:
                json.dump(metadata, f, indent=2)
            return True
        except Exception as e:
            logger.error(f"Failed to save metadata: {e}")
            return False

    def _load_metadata(self) -> Optional[Dict]:
        """Load node metadata from file"""
        try:
            with open(self.metadata_file, 'r') as f:
                return json.load(f)
        except Exception as e:
            logger.error(f"Failed to load metadata: {e}")
            return None

    def _cleanup(self):
        """Clean up existing node data"""
        if self.metadata_file.exists():
            self.metadata_file.unlink()
        
        nodes_dir = self.data_dir / "nodes"
        if nodes_dir.exists():
            shutil.rmtree(nodes_dir)

def main():
    """Main entry point"""
    parser = argparse.ArgumentParser(description="Rubix Node Manager - Python Implementation")
    parser.add_argument("--nodes", type=int, default=5, help="Number of transaction nodes to start (2-20)")
    parser.add_argument("--fresh", action="store_true", help="Fresh start - clean existing data")
    parser.add_argument("--restart", action="store_true", help="Restart using existing metadata")
    
    args = parser.parse_args()
    
    # Create configuration
    config = RubixConfig()
    
    # Create manager
    manager = RubixManager(config)
    
    try:
        if args.restart:
            logger.info("Restarting nodes using existing metadata...")
            success = manager.start_nodes(2, fresh=False)  # Default to 2 for restart
        else:
            logger.info(f"Starting {args.nodes} transaction nodes (fresh={args.fresh})...")
            success = manager.start_nodes(args.nodes, fresh=args.fresh)
        
        if success:
            logger.info("✓ Node startup completed successfully!")
        else:
            logger.error("✗ Node startup failed!")
            sys.exit(1)
            
    except Exception as e:
        logger.error(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
