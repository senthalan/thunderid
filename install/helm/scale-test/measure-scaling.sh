#!/usr/bin/env python3

import subprocess
import time
import re
import threading
import sys
import select
from datetime import datetime

# Configuration
NAMESPACE = "thunder-setup"
DEPLOYMENT = "thunder-deployment"
LABEL_SELECTOR = "app.kubernetes.io/name=thunder"

def run_cmd(cmd_list):
    """Runs a command and returns stdout."""
    result = subprocess.run(cmd_list, capture_output=True, text=True)
    return result.stdout.strip()

def get_current_replicas():
    cmd = ["kubectl", "get", "deployment", DEPLOYMENT, "-n", NAMESPACE, 
           "-o", "jsonpath={.spec.replicas}"]
    try:
        out = run_cmd(cmd)
        return int(out) if out else 0
    except ValueError:
        return 0

def wait_for_scale_up(initial_replicas):
    print(f"Waiting for scale up (current replicas: {initial_replicas})...")
    while True:
        current = get_current_replicas()
        if current > initial_replicas:
            t0 = time.time()
            print(f"\n[SCALE] Scale up detected! Replicas increased: {initial_replicas} -> {current}")
            print(f"[SCALE] Time: {datetime.fromtimestamp(t0).strftime('%H:%M:%S.%f')}")
            return t0, current
        time.sleep(0.01) # Check every ~10ms (plus command execution time)

def find_new_pod(initial_pods):
    """Polls for a new pod that wasn't in the initial list."""
    print("Waiting for new pod to be created...")
    while True:
        cmd = ["kubectl", "get", "pods", "-n", NAMESPACE, "-l", LABEL_SELECTOR, 
               "--sort-by=.metadata.creationTimestamp", "-o", "jsonpath={.items[*].metadata.name}"]
        current_pods = set(run_cmd(cmd).split())
        
        new_pods = current_pods - initial_pods
        if new_pods:
            new_pod = list(new_pods)[0]
            print(f"[POD] New pod detected: {new_pod}")
            return new_pod
            
        time.sleep(0.05) # Check frequently

def wait_for_pod_ready(pod_name):
    print(f"Waiting for pod {pod_name} to be ready...")
    cmd = ["kubectl", "wait", "--for=condition=ready", "pod", pod_name, "-n", NAMESPACE, "--timeout=300s"]
    subprocess.run(cmd, capture_output=True)
    ready_time = time.time()
    return ready_time

def get_pod_details(pod_name):
    """Fetches extraction timestamps for Scheduled, Created, Started."""
    # We need to get the full JSON to parse conditions and containerStatuses
    cmd = ["kubectl", "get", "pod", pod_name, "-n", NAMESPACE, "-o", "json"]
    try:
        out = run_cmd(cmd)
        import json
        pod_data = json.loads(out)
        
        details = {}
        
        # 1. Scheduled Time (PodScheduled condition)
        for cond in pod_data.get('status', {}).get('conditions', []):
            if cond['type'] == 'PodScheduled' and cond['status'] == 'True':
                details['scheduled'] = cond['lastTransitionTime']
                
        # 2. Container Started Time (from containerStatuses)
        # Assuming single container for now or taking the first one
        c_statuses = pod_data.get('status', {}).get('containerStatuses', [])
        if c_statuses:
            first_c = c_statuses[0]
            if 'running' in first_c.get('state', {}):
                details['started'] = first_c['state']['running']['startedAt']
                
        return details
    except Exception as e:
        print(f"Error fetching pod details: {e}")
        return {}


def parse_k8s_time(t_str):
    """Parses K8s timestamp string to float seconds."""
    if not t_str: return None
    # K8s time format: 2023-10-25T10:00:00Z
    # Python 3.7+ strptime handles %z usually, but Z might need replacement if not using dateutil
    # Simple fix for 'Z' -> '+0000'
    try:
        if t_str.endswith('Z'):
            t_str = t_str[:-1] + '+0000'
        dt = datetime.strptime(t_str, "%Y-%m-%dT%H:%M:%S%z")
        return dt.timestamp()
    except ValueError:
        return None

def monitor_logs(pod_name):
    print(f"Tailing logs for {pod_name}...")
    cmd = ["kubectl", "logs", "-f", pod_name, "-n", NAMESPACE]
    process = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, bufsize=1)
    
    start_tail = time.time()
    t_readiness_probe = None
    t_token_request = None
    
    print("Waiting for:\n 1. First Readiness Probe (GET /health/readiness)\n 2. First Token Request (POST /oauth2/token)")
    
    while True:
        # Timeout after 120s
        if time.time() - start_tail > 120:
            print("Timeout waiting for traffic logs.")
            process.terminate()
            return t_readiness_probe, t_token_request

        reads, _, _ = select.select([process.stdout], [], [], 1.0)
        if process.stdout in reads:
            line = process.stdout.readline()
            if not line: break
            
            # Check for Readiness Probe
            if not t_readiness_probe and "GET /health/readiness" in line:
                t_readiness_probe = time.time()
                print(f"[LOG] First Readiness Probe detected!")
                print(f"      {line.strip()}")

            # Check for Token Request
            if not t_token_request and "POST /oauth2/token" in line:
                t_token_request = time.time()
                print(f"[LOG] First Token Request detected!")
                print(f"      {line.strip()}")
                
            # If both found, we can exit early (or keep watching? usually we stop here)
            if t_readiness_probe and t_token_request:
                process.terminate()
                return t_readiness_probe, t_token_request
                
        if process.poll() is not None:
            break
            
    return t_readiness_probe, t_token_request

def main():
    print(f"--- Monitoring Scaling for {DEPLOYMENT} in {NAMESPACE} ---")
    
    # 1. Get initial state
    cmd = ["kubectl", "get", "pods", "-n", NAMESPACE, "-l", LABEL_SELECTOR, 
           "-o", "jsonpath={.items[*].metadata.name}"]
    initial_pods = set(run_cmd(cmd).split())
    initial_replicas = get_current_replicas()
    
    print(f"Initial Pods: {len(initial_pods)}")
    
    try:
        # 2. Wait for Scale Up
        t0, target_replicas = wait_for_scale_up(initial_replicas)
        
        # 3. Detect New Pod
        new_pod = find_new_pod(initial_pods)
        
        # 4. Wait for Ready
        t_ready = wait_for_pod_ready(new_pod)
        
        # 5. Monitor Logs for Readiness and Traffic
        t_probe, t_traffic = monitor_logs(new_pod)
        if t_traffic:
            # Fetch detailed timestamps
            details = get_pod_details(new_pod)

            print("\n" + "="*50)
            print("SCALING PERFORMANCE RESULTS")
            print("="*50)
            print(f"Scale Triggered:      {datetime.fromtimestamp(t0).strftime('%H:%M:%S.%f')}")

            print(f"Pod Ready (K8s):      {datetime.fromtimestamp(t_ready).strftime('%H:%M:%S.%f')}")
            
            if t_probe:
                print(f"First Readiness Log:  {datetime.fromtimestamp(t_probe).strftime('%H:%M:%S.%f')}")
            
            if t_traffic:
                print(f"First Token Request:  {datetime.fromtimestamp(t_traffic).strftime('%H:%M:%S.%f')}")

            print("-" * 50)
            
            print(f"Total Time to Ready:        {t_ready - t0:.3f}s")
            
            if t_traffic:
                print(f"Total Time to First Token:  {t_traffic - t0:.3f}s")
            print("="*50)

    except KeyboardInterrupt:
        print("\nStopping...")

if __name__ == "__main__":
    main()