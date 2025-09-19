#!/usr/bin/env python3
"""
Rubix Restart Setup Script

This script restarts Rubix nodes using existing metadata from node_metadata.json
It starts node processes and sets up the network without creating new DIDs.

Usage:
    python rubix_restart_setup.py
"""

import os
import sys
import json
import time
import shutil
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
        logging.FileHandler('rubix_restart.log'),
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
        self.node_startup_timeout = 120  # seconds
        self.default_priv_key_password = "mypassword"
        self.default_quorum_key_password = "mypassword"

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
    """Client utilities for communicating with Rubix nodes"""
    
    def __init__(self, base_url: str, node_dir: Optional[str] = None):
        self.base_url = base_url
        self.node_dir = node_dir
        self.session = requests.Session()
        self.session.timeout = 30

    def wait_for_node(self, timeout: int = 120) -> bool:
        """Wait for node to be ready using rubixgoplatform getalldid only."""
        # Derive port from base_url
        try:
            port_str = self.base_url.rsplit(":", 1)[-1]
            port = int(port_str)
        except Exception:
            logger.error(f"  ✗ Could not parse port from {self.base_url}")
            return False

        if not self.node_dir:
            logger.error("  ✗ Node directory not provided; cannot run getalldid")
            return False

        logger.info(f"  Waiting for node (CLI getalldid) on port {port} (timeout: {timeout}s)...")
        start_time = time.time()
        while time.time() - start_time < timeout:
            try:
                # Choose executable per platform
                exe = "rubixgoplatform.exe" if platform.system() == "Windows" else "./rubixgoplatform"
                cmd = [exe, "getalldid", "-port", str(port)]
                result = subprocess.run(
                    cmd,
                    cwd=self.node_dir,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.STDOUT,
                    text=True,
                )

                out = result.stdout or ""
                if result.returncode == 0 and ("Got all DID successfully" in out or "Address :" in out):
                    logger.info(f"  ✓ Node at port {port} is ready (getalldid)")
                    return True
            except Exception:
                # Ignore and retry
                pass

            time.sleep(2)

        logger.error(f"  ✗ Node at port {port} failed to start within {timeout}s (getalldid)")
        return False

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

class RubixRestartManager:
    """Manager for restarting Rubix nodes using existing metadata"""
    
    def __init__(self, config: RubixConfig):
        self.config = config
        self.nodes: Dict[str, NodeInfo] = {}
        self.data_dir = Path(config.data_dir)
        self.metadata_file = self.data_dir / "node_metadata.json"
        self.rubix_path = self.data_dir / "rubixgoplatform"

    def restart_nodes(self) -> bool:
        """Restart nodes using existing metadata"""
        
        # Phase 1: Load and validate metadata
        logger.info("\n================== PHASE 1: Load Metadata ==================")
        if not self._load_metadata():
            return False
        
        # Phase 2: Verify node directories and binaries
        logger.info("\n================== PHASE 2: Node Directory Verification ==================")
        if not self._verify_node_directories():
            return False
        
        # Phase 3: Start node processes
        logger.info("\n================== PHASE 3: Starting Node Processes ==================")
        logger.info(f"Starting {len(self.nodes)} nodes from metadata...")
        
        started_nodes = 0
        for node_id, node_info in self.nodes.items():
            node_type = "quorum" if node_info.is_quorum else "transaction"
            logger.info(f"[{node_id}] Starting {node_type} node on port {node_info.server_port}")
            
            # Start node process
            if not self._start_node_process(node_info):
                logger.error(f"  ✗ ERROR: Failed to start {node_id}")
                return False
            
            # Wait for node to be ready (prefer CLI getalldid in node directory)
            abs_data_dir = os.path.abspath(str(self.data_dir))
            node_dir = os.path.join(abs_data_dir, "nodes", node_info.id)
            client = RubixClient(f"http://localhost:{node_info.server_port}", node_dir=node_dir)
            if not client.wait_for_node(self.config.node_startup_timeout):
                logger.error(f"  ✗ ERROR: {node_id} failed to become ready")
                return False
            
            started_nodes += 1
            logger.info(f"  ✓ {node_id} started successfully")
        
        logger.info(f"Node startup complete: {started_nodes}/{len(self.nodes)} nodes started")
        
        # Phase 4: DID Registration
        logger.info("\n================== PHASE 4: DID Registration ==================")
        logger.info(f"Registering {len(self.nodes)} existing DIDs with the network...")
        
        registration_success = 0
        for node_id, node_info in self.nodes.items():
            if not node_info.did:
                logger.warning(f"  ⚠ WARNING: {node_id} has no DID in metadata, skipping registration")
                continue
            
            node_type = "quorum" if node_info.is_quorum else "transaction"
            did_display = node_info.did[:16] + "..." if len(node_info.did) > 16 else node_info.did
            logger.info(f"[{node_id}] Registering {node_type} node DID: {did_display}")
            
            client = RubixClient(f"http://localhost:{node_info.server_port}")
            if client.register_did(node_info.did, self.config.default_priv_key_password):
                registration_success += 1
            else:
                logger.error(f"  ✗ ERROR: Failed to register DID for {node_id}")
        
        logger.info(f"DID registration complete: {registration_success}/{len(self.nodes)} nodes registered")
        
        # Phase 5: Build and Distribute Quorum List
        logger.info("\n================== PHASE 5: Quorum List Distribution ==================")
        
        # Build quorum list from metadata
        quorum_list = []
        quorum_count = 0
        for node_id, node_info in self.nodes.items():
            if node_info.is_quorum and node_info.did:
                quorum_list.append({
                    "type": 2,
                    "address": node_info.did
                })
                quorum_count += 1
        
        logger.info(f"Built quorum list with {len(quorum_list)} members from metadata:")
        for i, q in enumerate(quorum_list):
            addr_display = q["address"][:16] + "..." if len(q["address"]) > 16 else q["address"]
            logger.info(f"  [{i+1}] Quorum DID: {addr_display} (Type: {q['type']})")
        
        # Distribute to all nodes
        quorum_add_success = 0
        for node_id, node_info in self.nodes.items():
            node_type = "quorum" if node_info.is_quorum else "transaction"
            logger.info(f"[{node_id}] Adding quorum list to {node_type} node...")
            
            client = RubixClient(f"http://localhost:{node_info.server_port}")
            if client.add_quorum(quorum_list):
                quorum_add_success += 1
            else:
                logger.error(f"  ✗ ERROR: Failed to add quorum to {node_id}")
        
        logger.info(f"Quorum distribution complete: {quorum_add_success}/{len(self.nodes)} nodes configured")
        
        # Phase 6: Setup Quorum
        logger.info("\n================== PHASE 6: Quorum Setup ==================")
        logger.info(f"Setting up {quorum_count} quorum nodes with quorum-specific configuration...")
        
        quorum_setup_success = 0
        for node_id, node_info in self.nodes.items():
            if node_info.is_quorum and node_info.did:
                client = RubixClient(f"http://localhost:{node_info.server_port}")
                did_display = node_info.did[:16] + "..." if len(node_info.did) > 16 else node_info.did
                logger.info(f"[{node_id}] Setting up quorum configuration (DID: {did_display})...")
                
                if client.setup_quorum(
                    node_info.did,
                    self.config.default_quorum_key_password,
                    self.config.default_priv_key_password
                ):
                    quorum_setup_success += 1
                else:
                    logger.error(f"  ✗ ERROR: Failed to setup quorum for {node_id}")
        
        logger.info(f"Quorum setup complete: {quorum_setup_success}/{quorum_count} quorum nodes configured")
        
        # Phase 7: Token Generation
        logger.info("\n================== PHASE 7: Token Generation ==================")
        logger.info(f"Generating 100 test RBT tokens for all {len(self.nodes)} nodes...")
        
        token_gen_success = 0
        for node_id, node_info in self.nodes.items():
            if not node_info.did:
                logger.warning(f"  ⚠ WARNING: {node_id} has no DID, skipping token generation")
                continue
            
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
        
        # Phase 8: Summary
        logger.info("\n================== RESTART COMPLETE ==================")
        logger.info("Summary:")
        logger.info(f"  - Nodes started: {started_nodes}/{len(self.nodes)}")
        logger.info(f"  - DIDs registered: {registration_success}/{len(self.nodes)}")
        logger.info(f"  - Quorum distributed: {quorum_add_success}/{len(self.nodes)}")
        logger.info(f"  - Quorum setup: {quorum_setup_success}/{quorum_count}")
        logger.info(f"  - Tokens generated: {token_gen_success}/{len(self.nodes)}")
        
        if (started_nodes < len(self.nodes) or 
            registration_success < len(self.nodes) or 
            quorum_add_success < len(self.nodes) or 
            token_gen_success < len(self.nodes)):
            logger.warning("⚠ WARNING: Some operations failed. Check logs above for details.")
            return False
        else:
            logger.info("✓ All nodes successfully restarted and configured!")
            return True

    def _load_metadata(self) -> bool:
        """Load node metadata from file"""
        if not self.metadata_file.exists():
            logger.error(f"✗ ERROR: Metadata file not found: {self.metadata_file}")
            logger.error("Please run a fresh setup first or ensure the metadata file exists")
            return False
        
        try:
            with open(self.metadata_file, 'r') as f:
                metadata = json.load(f)
            
            if not metadata:
                logger.error("✗ ERROR: Metadata file is empty")
                return False
            
            # Convert to NodeInfo objects
            for node_id, node_data in metadata.items():
                try:
                    node_info = NodeInfo.from_dict(node_data)
                    self.nodes[node_id] = node_info
                except Exception as e:
                    logger.error(f"✗ ERROR: Invalid metadata for {node_id}: {e}")
                    return False
            
            logger.info(f"✓ Loaded metadata for {len(self.nodes)} nodes")
            
            # Log summary
            quorum_nodes = sum(1 for node in self.nodes.values() if node.is_quorum)
            transaction_nodes = len(self.nodes) - quorum_nodes
            logger.info(f"  - Quorum nodes: {quorum_nodes}")
            logger.info(f"  - Transaction nodes: {transaction_nodes}")
            
            # Validate DIDs exist
            nodes_with_dids = sum(1 for node in self.nodes.values() if node.did)
            if nodes_with_dids == 0:
                logger.error("✗ ERROR: No nodes have DIDs in metadata")
                return False
            elif nodes_with_dids < len(self.nodes):
                logger.warning(f"⚠ WARNING: Only {nodes_with_dids}/{len(self.nodes)} nodes have DIDs")
            
            return True
            
        except Exception as e:
            logger.error(f"✗ ERROR: Failed to load metadata: {e}")
            return False

    def _verify_node_directories(self) -> bool:
        """Verify node directories and binaries exist (for restart scenario)"""
        logger.info("Verifying existing node directories and binaries...")
        
        if platform.system() == "Windows":
            rubix_bin = "rubixgoplatform.exe"
            ipfs_bin = "ipfs.exe"
        else:
            rubix_bin = "rubixgoplatform"
            ipfs_bin = "ipfs"
        
        missing_nodes = []
        missing_binaries = []
        
        for node_id, node_info in self.nodes.items():
            # Check node directory exists
            node_dir = self.data_dir / "nodes" / node_id
            if not node_dir.exists():
                missing_nodes.append(node_id)
                continue
            
            # Check required binaries exist in node directory
            required_files = [
                node_dir / rubix_bin,
                node_dir / ipfs_bin,
                node_dir / "testswarm.key"
            ]
            
            for file_path in required_files:
                if not file_path.exists():
                    missing_binaries.append(f"{node_id}: {file_path.name}")
        
        # Report issues
        if missing_nodes:
            logger.error(f"✗ ERROR: Missing node directories: {', '.join(missing_nodes)}")
            logger.error("These nodes were not previously set up or directories were deleted")
            return False
        
        if missing_binaries:
            logger.error(f"✗ ERROR: Missing binaries in node directories:")
            for binary in missing_binaries:
                logger.error(f"  - {binary}")
            logger.error("Run a fresh setup first to copy binaries to node directories")
            return False
        
        logger.info(f"✓ All {len(self.nodes)} node directories and binaries verified")
        return True

    def _start_node_process(self, node_info: NodeInfo) -> bool:
        """Start a single node process using existing binaries in node directory"""
        
        # Extract node index from ID (e.g., "node0" -> 0)
        try:
            index = int(node_info.id.replace("node", ""))
        except ValueError:
            logger.error(f"✗ ERROR: Invalid node ID format: {node_info.id}")
            return False
        
        # Get node directory (where binaries are already copied)
        abs_data_dir = os.path.abspath(str(self.data_dir))
        node_dir = os.path.join(abs_data_dir, "nodes", node_info.id)
        
        # Platform-specific binary names
        if platform.system() == "Windows":
            rubix_bin = "rubixgoplatform.exe"
        else:
            rubix_bin = "rubixgoplatform"
        
        # NOTE: We don't need build directory for restart!
        # Binaries are already in node directories from previous setup
        # We just verify they exist (done in _verify_node_directories)
        
        # Build command arguments (using ports from metadata)
        args = [
            "run",
            "-p", node_info.id,
            "-n", str(index),
            "-s",
            "-port", str(node_info.server_port),
            "-testNet",
            "-grpcPort", str(node_info.grpc_port)
        ]
        
        # Create platform-specific command
        try:
            if platform.system() == "Windows":
                # Create batch file
                window_title = f"Rubix Node {node_info.id} - Port {node_info.server_port}"
                
                batch_content = f"""@echo off
title {window_title}
echo Starting {node_info.id} on port {node_info.server_port}...
echo Node directory: {node_dir}
echo.
cd /d "{node_dir}"
if not exist "{rubix_bin}" (
    echo ERROR: {rubix_bin} not found in node directory
    pause > nul
    exit /b 1
)
if not exist "{ipfs_bin}" (
    echo ERROR: {ipfs_bin} not found in node directory
    pause > nul
    exit /b 1
)
if not exist "testswarm.key" (
    echo ERROR: testswarm.key not found in node directory
    pause > nul
    exit /b 1
)
"{rubix_bin}" {' '.join(args)}
echo.
echo Node stopped. Press any key to close this window...
pause > nul"""
                
                # Write batch file
                batch_path = self.data_dir / f"node_{node_info.id}.bat"
                batch_path.write_text(batch_content)
                
                # Start batch file in new window
                cmd = ["cmd", "/c", "start", "", str(batch_path)]
                
            else:
                # Linux/Mac: use tmux session
                session_name = f"rubix-node-{node_info.id}"
                node_command = f"cd {node_dir} && ./{rubix_bin} {' '.join(args)}"
                cmd = ["tmux", "new-session", "-d", "-s", session_name, node_command]
            
            # Environment variables
            env = os.environ.copy()
            env.update({
                "RUBIX_NODE_DIR": str(node_dir),
                "RUBIX_NODE_ID": node_info.id
            })
            
            # Log command details
            logger.info(f"  Starting {node_info.id} from directory: {node_dir}")
            logger.info(f"  Command: {rubix_bin} {' '.join(args)}")
            
            # Start process
            process = subprocess.Popen(cmd, env=env)
            node_info.process = process
            
            # Give node time to boot
            time.sleep(30)
            return True
            
        except Exception as e:
            logger.error(f"  ✗ Failed to start node process: {e}")
            return False

def main():
    """Main entry point"""
    logger.info("=== Rubix Restart Setup Script ===")
    logger.info("This script restarts nodes using existing metadata")
    
    # Create configuration
    config = RubixConfig()
    
    # Create restart manager
    manager = RubixRestartManager(config)
    
    try:
        success = manager.restart_nodes()
        
        if success:
            logger.info("✓ Node restart completed successfully!")
        else:
            logger.error("✗ Node restart failed!")
            sys.exit(1)
            
    except KeyboardInterrupt:
        logger.info("\nRestart interrupted by user")
        sys.exit(1)
    except Exception as e:
        logger.error(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
