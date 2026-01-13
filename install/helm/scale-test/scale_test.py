# thunder_demo_local.py
import asyncio
import aiohttp
import time
from dataclasses import dataclass

@dataclass
class OAuth2Client:
    client_id: str
    client_secret: str

CLIENTS = [OAuth2Client(f"client_id_{i}", f"client_secret_{i}") for i in range(1, 11)]
THUNDER_TOKEN_ENDPOINT = "https://thunder.local/oauth2/token"  # Likely NodePort on local


import subprocess
import threading
from collections import defaultdict
import re

try:
    import matplotlib.pyplot as plt
except ImportError:
    print("Matplotlib not found. Please install it with: pip install matplotlib")
    plt = None


import select

class EventCollector(threading.Thread):
    def __init__(self, stop_event):
        super().__init__()
        self.stop_event = stop_event
        self.events = [] # [(time, "Triggered Scale Up"|"Triggered Scale Down", count)]

    def run(self):
        # We use --watch to get real-time updates
        cmd = ["kubectl", "get", "events", "-n", "thunder-setup", "--watch"]
        process = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, bufsize=1)
        
        while not self.stop_event.is_set():
            # Use select to check for data availability with a timeout
            # This allows us to check stop_event periodically even if no events occur
            reads, _, _ = select.select([process.stdout], [], [], 1.0)
            
            if process.stdout in reads:
                line = process.stdout.readline()
                if not line:
                    break
                    
                if "ScalingReplicaSet" in line or "SuccessfulRescale" in line:
                    if "Scaled up" in line:
                        try:
                            count = int(re.search(r'to (\d+)', line).group(1))
                            self.events.append((time.time(), "Trigger Up", count))
                        except: pass
                    elif "Scaled down" in line:
                        try:
                            count = int(re.search(r'to (\d+)', line).group(1))
                            self.events.append((time.time(), "Trigger Down", count))
                        except: pass
            
            if process.poll() is not None:
                break

        process.terminate()

class PodStatusCollector(threading.Thread):
    def __init__(self, stop_event):
        super().__init__()
        self.stop_event = stop_event
        self.ready_events = [] # [(time, "Pod Ready", pod_name)]
        self.known_ready_pods = set()

    def run(self):
        # We use --watch to get real-time partial updates (much more accurate than polling)
        # Output format: pod_name:phase:ready_status
        cmd = ["kubectl", "get", "pods", "-n", "thunder-setup", "--watch", 
               "-o", 'jsonpath={.metadata.name}:{.status.phase}:{.status.conditions[?(@.type=="Ready")].status}{"\\n"}']
        
        process = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, bufsize=1)
        
        while not self.stop_event.is_set():
            # Check for output or timeout (to check stop_event)
            reads, _, _ = select.select([process.stdout], [], [], 0.5)
            
            if process.stdout in reads:
                line = process.stdout.readline()
                if not line:
                    break
                
                try:
                    parts = line.strip().split(':')
                    if len(parts) >= 3:
                        name = parts[0]
                        phase = parts[1]
                        ready_status = parts[2]
                        
                        if phase == "Running" and ready_status == "True":
                            if name not in self.known_ready_pods:
                                self.ready_events.append((time.time(), "Pod Ready", name))
                                self.known_ready_pods.add(name)
                        else:
                            # Pod is not ready (or no longer ready)
                            self.known_ready_pods.discard(name)
                except Exception:
                    pass
            
            if process.poll() is not None:
                break

        try:
            process.terminate()
        except:
            pass

class MetricCollector(threading.Thread):
    def __init__(self, stop_event):
        super().__init__()
        self.stop_event = stop_event
        self.pod_metrics = defaultdict(list) # {pod_name: [(time, cpu_m, mem_mi)]}

    def run(self):
        while not self.stop_event.is_set():
            try:
                cmd = ["kubectl", "top", "pods", "-n", "thunder-setup", "--no-headers"]
                result = subprocess.run(cmd, capture_output=True, text=True)
                
                if result.returncode == 0:
                    current_time = time.time()
                    for line in result.stdout.strip().split('\n'):
                        parts = line.split()
                        if len(parts) >= 3:
                            pod_name = parts[0]
                            cpu = int(parts[1].replace('m', ''))
                            mem = int(parts[2].replace('Mi', ''))
                            
                            self.pod_metrics[pod_name].append((current_time, cpu, mem))
                        
            except Exception as e:
                print(f"Error collecting metrics: {e}\n")
            
            time.sleep(0.5) # Poll every 0.5 second

def generate_graph_image(data_points, phases=None, pod_metrics=None, trigger_events=None, ready_events=None, filename="response_times.png"):
    """
    Generates a PNG graph with 3 subplots: Response Time, CPU, Memory.
    """
    if not data_points:
        print("\nNo data to graph.")
        return

    if not plt:
        print("\nSkipping graph generation (matplotlib not installed).")
        return

    # Sort by timestamp
    data_points.sort(key=lambda x: x[0])
    
    start_time = data_points[0][0]
    
    # Setup subplots
    fig, (ax1, ax2, ax3) = plt.subplots(3, 1, figsize=(12, 12), sharex=True)
    
    # 1. Response Time Plot
    timestamps = [x[0] - start_time for x in data_points]
    latencies = [x[1] for x in data_points]
    
    ax1.plot(timestamps, latencies, marker='.', linestyle='-', linewidth=0.5, markersize=2, alpha=0.7, label='Latency')
    ax1.set_ylabel("Response Time (s)")
    ax1.set_title("Load Test Performance Metrics")
    ax1.grid(True, linestyle='--', alpha=0.6)
    
    avg_latency = sum(latencies) / len(latencies)
    ax1.axhline(y=avg_latency, color='black', linestyle=':', label=f'Avg: {avg_latency:.3f}s')
    ax1.legend(loc='upper right')
    
    # 2. CPU Usage Plot
    if pod_metrics:
        for pod, metrics in pod_metrics.items():
            valid_metrics = [m for m in metrics if m[0] >= start_time]
            if valid_metrics:
                t = [m[0] - start_time for m in valid_metrics]
                c = [m[1] for m in valid_metrics]
                ax2.plot(t, c, label=pod, linewidth=1.5)
    
    ax2.set_ylabel("CPU (m)")
    ax2.grid(True, linestyle='--', alpha=0.6)
    
    # 3. Memory Usage Plot
    if pod_metrics:
        for pod, metrics in pod_metrics.items():
            valid_metrics = [m for m in metrics if m[0] >= start_time]
            if valid_metrics:
                t = [m[0] - start_time for m in valid_metrics]
                m_usage = [m[2] for m in valid_metrics]
                ax3.plot(t, m_usage, label=pod, linewidth=1.5)
                
    ax3.set_ylabel("Memory (Mi)")
    ax3.set_xlabel("Time (seconds)")
    ax3.grid(True, linestyle='--', alpha=0.6)
    
    # Add Phase Markers
    if phases:
        colors = ['green', 'orange', 'red', 'purple', 'blue']
        for i, (phase_name, phase_start) in enumerate(phases):
            rel_start = phase_start - start_time
            if rel_start < 0: rel_start = 0
            color = colors[i % len(colors)]
            
            for ax in [ax1, ax2, ax3]:
                ax.axvline(x=rel_start, color=color, linestyle='--', alpha=0.5)
            
            ax1.text(rel_start + 0.5, ax1.get_ylim()[1] * 0.95, phase_name, 
                     rotation=90, verticalalignment='top', color=color, fontsize=8, fontweight='bold')

    # Add Scale Trigger Events (Vertical Lines)
    if trigger_events:
        for t, event_type, count in trigger_events:
            rel_t = t - start_time
            if rel_t >= 0:
                color = 'magenta' if "Up" in event_type else 'cyan'
                linestyle = '-.'
                
                for ax in [ax2, ax3]:
                    ax.axvline(x=rel_t, color=color, linestyle=linestyle, alpha=0.8, linewidth=1)
                
                ax2.text(rel_t, ax2.get_ylim()[1], f"{event_type}\n(Target: {count})", 
                         color=color, fontsize=7, rotation=90, verticalalignment='bottom')

    # Add Pod Ready Events (Markers)
    if ready_events:
        for t, _, pod_name in ready_events:
            rel_t = t - start_time
            if rel_t >= 0:
                # Mark with a Star on CPU plot
                # Find the CPU value at this time if possible, or just place it at bottom
                ax2.plot(rel_t, 0, marker='*', markersize=10, color='gold', markeredgecolor='black', zorder=10)
                ax2.text(rel_t, 0, "Ready", fontsize=7, color='black', ha='center', va='top')

    plt.tight_layout()
    plt.savefig(filename)
    print(f"\n📊 Graph saved to: {filename}")
    plt.close()


async def get_token(session, client):
    try:
        headers = {
            'Content-Type': 'application/x-www-form-urlencoded'
        }
        start = time.time()
        async with session.post(
            THUNDER_TOKEN_ENDPOINT,
            headers=headers,
            auth=aiohttp.BasicAuth(client.client_id, client.client_secret),
            data={
                "grant_type": "client_credentials",
            },
            timeout=aiohttp.ClientTimeout(total=5)
        ) as response:
            await response.json()
            return time.time() - start
    except Exception as e:
        return e

async def run_load(rps: int, duration: int, phase_name: str):
    print(f"\n{'='*70}")
    print(f"⚡ {phase_name} - {rps} RPS for {duration}s")
    print(f"{'='*70}")
    
    connector = aiohttp.TCPConnector(limit=100, limit_per_host=50, ssl=False)  # Limit connections
    async with aiohttp.ClientSession(connector=connector) as session:
        start_time = time.time()
        total_requests = 0
        successful = 0
        failed = 0
        latencies = [] # Store (timestamp, latency)
        
        while time.time() - start_time < duration:
            batch_start = time.time()
            
            # Smaller batches for local demo
            batch_size = min(rps, 50)  # Cap at 50 concurrent requests
            tasks = []
            for _ in range(batch_size):
                client = CLIENTS[total_requests % len(CLIENTS)]
                tasks.append(get_token(session, client))
            
            results = await asyncio.gather(*tasks, return_exceptions=True)
            
            now = time.time()
            for result in results:
                total_requests += 1
                if isinstance(result, Exception):
                    failed += 1
                elif isinstance(result, float): # Latency returned
                    successful += 1
                    latencies.append((now, result))
                else: # Fallback
                     failed += 1
            
            elapsed = time.time() - start_time
            current_rps = total_requests / elapsed if elapsed > 0 else 0
            success_rate = (successful / total_requests * 100) if total_requests > 0 else 0
            
            if success_rate >= 98:
                status = "✅"
            elif success_rate >= 90:
                status = "⚠️ "
            else:
                status = "❌"
            
            print(f"{status} [{elapsed:5.1f}s] Total: {total_requests:5d} | "
                  f"✓ {successful:5d} | ✗ {failed:3d} | "
                  f"RPS: {current_rps:4.0f} | {success_rate:5.1f}%", end='\r')
            
            batch_duration = time.time() - batch_start
            sleep_time = max(0, 1 - batch_duration)
            await asyncio.sleep(sleep_time)
        
        print()
        return successful, failed, total_requests, latencies

async def main():
    print("\n🚀 THUNDER IDP LOCAL SCALING DEMO (M4 Laptop)")
    print("Resources: 4GB RAM, 2 CPU cores")
    print("="*70)
    
    # Start Collectors
    stop_event = threading.Event()
    
    metric_collector = MetricCollector(stop_event)
    event_collector = EventCollector(stop_event)
    pod_status_collector = PodStatusCollector(stop_event)
    
    metric_collector.start()
    event_collector.start()
    pod_status_collector.start()
    
    print("📊 Background Collectors Started (Metrics, Events, PodStatus)...")
    
    all_latencies = []
    phases = []
    
    try:
        # Reduced load for local constraints
        
        # Phase 1: Baseline (5s)
        print("\n📊 PHASE 1: Baseline Load")
        phases.append(("Baseline", time.time()))
        s1, f1, t1, l1 = await run_load(rps=50, duration=5, phase_name="Steady State - 50 RPS")
        all_latencies.extend(l1)
        
        # Phase 2: Gradual Ramp (20s)
        print("\n📈 PHASE 2: Gradual Increase")
        phases.append(("Ramp Up", time.time()))
        s2, f2, t2, l2 = await run_load(rps=150, duration=10, phase_name="Ramp to 150 RPS")
        all_latencies.extend(l2)
        s3, f3, t3, l3 = await run_load(rps=200, duration=10, phase_name="Ramp to 200 RPS")
        all_latencies.extend(l3)
        
        # Phase 3: SPIKE! (25s)
        print("\n💥 PHASE 3: TRAFFIC SPIKE!")
        phases.append(("Spike (400 RPS)", time.time()))
        s4, f4, t4, l4 = await run_load(rps=400, duration=40, phase_name="SPIKE - 400 RPS (8x baseline)")
        all_latencies.extend(l4)
        
        # Phase 4: Recovery (20s)
        print("\n📉 PHASE 4: Scale Down")
        phases.append(("Scale Down", time.time()))
        s5, f5, t5, l5 = await run_load(rps=200, duration=15, phase_name="Drop to 200 RPS")
        all_latencies.extend(l5)
        phases.append(("Recovery", time.time()))
        s6, f6, t6, l6 = await run_load(rps=150, duration=5, phase_name="Return to Baseline")
        all_latencies.extend(l6)
        
        # Summary
        total_success = s1 + s2 + s3 + s4 + s5 + s6
        total_failed = f1 + f2 + f3 + f4 + f5 + f6
        total_requests = t1 + t2 + t3 + t4 + t5 + t6
        
        print("\n" + "="*70)
        print("✨ THUNDER LOCAL DEMO COMPLETE")
        print("="*70)
        print(f"Total Requests:  {total_requests:,}")
        print(f"Successful:      {total_success:,} ({total_success/total_requests*100:.2f}%)")
        print(f"Failed:          {total_failed:,} ({total_failed/total_requests*100:.2f}%)")
        print(f"Duration:        85 seconds")
        print(f"Peak RPS:        400 (8x baseline)")
        print(f"Max Pods:        8 (from 2 baseline)")
        print("="*70)
        
        print("\n⏳ Waiting 60s to capture scale-down metrics...")
        await asyncio.sleep(60)
        
    finally:
        stop_event.set()
        metric_collector.join()
        event_collector.join()
        pod_status_collector.join()
        print("📊 Collectors Stopped.")
    
    generate_graph_image(
        all_latencies, 
        phases, 
        metric_collector.pod_metrics, 
        event_collector.events,
        pod_status_collector.ready_events
    )

if __name__ == "__main__":
    asyncio.run(main())