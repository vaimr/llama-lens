"""Direct backend API inspection script."""
import json
import subprocess
import sys
import time
import os

import httpx

def main():
    base_dir = "/workspace/llama-lens"
    
    # Start backend server
    print("Starting backend server on port 8000...")
    backend_proc = subprocess.Popen(
        ["python3", "-m", "uvicorn", "backend.main:app", 
         "--host", "0.0.0.0", "--port", "8000"],
        cwd=base_dir,
        env={**os.environ, "PYTHONPATH": base_dir},
        stderr=subprocess.PIPE,
        stdout=subprocess.PIPE,
    )
    time.sleep(4)
    
    try:
        # Test the API
        print("\n" + "="*80)
        print("Testing /api/hosts endpoint")
        print("="*80)
        try:
            resp = httpx.get("http://localhost:8000/api/hosts", timeout=10)
            print(f"Status: {resp.status_code}")
            hosts = resp.json()
            print(f"Response: {json.dumps(hosts, indent=2, default=str)}")
        except Exception as e:
            print(f"Error: {e}")
        
        print("\n" + "="*80)
        print("Testing /api/hosts/ai/overview endpoint")
        print("="*80)
        try:
            resp = httpx.get("http://localhost:8000/api/hosts/ai/overview", timeout=10)
            print(f"Status: {resp.status_code}")
            overview = resp.json()
            print(json.dumps(overview, indent=2, default=str))
            
            # Deep analysis
            if "host_metrics" in overview:
                hm = overview["host_metrics"]
                print("\n" + "="*60)
                print("host_metrics DEEP ANALYSIS")
                print("="*60)
                
                print(f"\nAll keys: {sorted(hm.keys())}")
                
                for key in sorted(hm.keys()):
                    val = hm[key]
                    if isinstance(val, dict):
                        print(f"\n  {key}: dict")
                        for k2, v2 in val.items():
                            if isinstance(v2, (dict, list)):
                                print(f"    {k2}: {type(v2).__name__} = {json.dumps(v2, default=str)[:200]}")
                            else:
                                print(f"    {k2}: {type(v2).__name__} = {repr(v2)[:100]}")
                    elif isinstance(val, list):
                        print(f"\n  {key}: list[{len(val)}]")
                        for i, item in enumerate(val[:5]):
                            if isinstance(item, dict):
                                print(f"    [{i}]: dict keys={list(item.keys())}")
                            else:
                                print(f"    [{i}]: {type(item).__name__} = {repr(item)[:100]}")
                    else:
                        print(f"\n  {key}: {type(val).__name__} = {repr(val)[:200]}")
                
                # Process analysis
                print("\n" + "="*60)
                print("process field")
                print("="*60)
                process = hm.get("process")
                print(f"Type: {type(process).__name__}")
                if isinstance(process, dict):
                    print(f"Keys: {list(process.keys())}")
                    proc_list = process.get("list", [])
                    print(f"list type: {type(proc_list).__name__}")
                    print(f"list length: {len(proc_list)}")
                    for k, proc in enumerate(proc_list):
                        print(f"\n  Process {k}:")
                        print(f"  Keys: {sorted(proc.keys())}")
                        for pk, pv in proc.items():
                            if isinstance(pv, str) and len(pv) > 100:
                                print(f"    {pk}: {pv[:100]}...")
                            else:
                                print(f"    {pk}: {repr(pv)[:200]}")
                elif isinstance(process, list):
                    print(f"Length: {len(process)}")
                    for k, proc in enumerate(process):
                        print(f"  Process {k}: {type(proc).__name__} = {repr(proc)[:200]}")
                else:
                    print(f"Value: {repr(process)}")
                
                # _model_paths
                print("\n" + "="*60)
                print("_model_paths")
                print("="*60)
                model_paths = hm.get("_model_paths")
                print(f"Type: {type(model_paths).__name__}")
                print(f"Value: {json.dumps(model_paths, indent=2, default=str)}")
                
                # _mmproj_paths
                print("\n" + "="*60)
                print("_mmproj_paths")
                print("="*60)
                mmproj_paths = hm.get("_mmproj_paths")
                print(f"Type: {type(mmproj_paths).__name__}")
                print(f"Value: {json.dumps(mmproj_paths, indent=2, default=str)}")
                
                # Service
                print("\n" + "="*60)
                print("service")
                print("="*60)
                service = hm.get("service")
                print(f"Type: {type(service).__name__}")
                print(f"Value: {json.dumps(service, indent=2, default=str)}")
                
                # Reachable
                print(f"\nreachable: {hm.get('reachable')}")
                
                # Llama model info
                print("\n" + "="*60)
                print("llama model")
                print("="*60)
                llama = overview.get("llama", {})
                model = llama.get("model", {})
                print(f"online: {llama.get('online')}")
                print(f"path: {model.get('path', 'N/A')}")
                print(f"name: {model.get('name', 'N/A')}")
                print(f"mmproj_path: {model.get('mmproj_path', 'N/A')}")
                print(f"file_size: {model.get('file_size', 'N/A')}")
                
                # Host info
                host = overview.get("host", {})
                print(f"\nhost.id: {host.get('id')}")
                print(f"host.name: {host.get('name')}")
                
        except Exception as e:
            print(f"Error: {e}")
            import traceback
            traceback.print_exc()
        
        # Also test WebSocket directly
        print("\n" + "="*80)
        print("Testing WebSocket /ws/hosts/ai directly")
        print("="*80)
        try:
            import asyncio
            import websockets
            
            async def test_ws():
                uri = "ws://localhost:8000/ws/hosts/ai"
                async with websockets.connect(uri) as ws:
                    for i in range(5):
                        msg = await asyncio.wait_for(ws.recv(), timeout=5)
                        data = json.loads(msg)
                        print(f"\n--- WS Message #{i+1} ---")
                        print(json.dumps(data, indent=2, default=str))
                        
                        if "host_metrics" in data:
                            hm = data["host_metrics"]
                            print(f"\nprocess type: {type(hm.get('process')).__name__}")
                            process = hm.get("process")
                            if isinstance(process, dict):
                                print(f"process keys: {list(process.keys())}")
                                pl = process.get("list", [])
                                print(f"process.list length: {len(pl)}")
                                if pl:
                                    print(f"First process keys: {sorted(pl[0].keys())}")
                                    print(f"First process: {json.dumps(pl[0], indent=2, default=str)}")
                            print(f"_model_paths: {json.dumps(hm.get('_model_paths'), indent=2, default=str)}")
                            print(f"_mmproj_paths: {json.dumps(hm.get('_mmproj_paths'), indent=2, default=str)}")
                            print(f"service: {json.dumps(hm.get('service'), indent=2, default=str)}")
                            print(f"reachable: {hm.get('reachable')}")
                        
                        await asyncio.sleep(1.5)
            
            asyncio.run(test_ws())
        except Exception as e:
            print(f"WebSocket error: {e}")
            import traceback
            traceback.print_exc()
    
    finally:
        print("\nShutting down server...")
        backend_proc.terminate()
        try:
            backend_proc.wait(timeout=5)
        except:
            backend_proc.kill()
        print("Done.")

if __name__ == "__main__":
    main()
