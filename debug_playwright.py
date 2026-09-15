"""
Playwright debug v5: Use page.on("websocket") properly + direct API dump.
The native WS event captures the connection; we extract data from it.
"""
import json
import time
import asyncio

def run():
    from playwright.sync_api import sync_playwright
    
    ws_connections = []
    ws_data_messages = []
    
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(viewport={"width": 1920, "height": 1080})
        page = context.new_page()
        
        console_msgs = []
        page.on("console", lambda m: console_msgs.append(f"[{m.type}] {m.text[:200]}"))
        page.on("pageerror", lambda e: console_msgs.append(f"[ERR] {str(e)[:200]}"))
        
        # Set up WS listener BEFORE navigation
        print("[1/5] Setting up WS listener...")
        
        def handle_websocket(ws):
            url = ws.url
            ws_connections.append(url)
            print(f"  [WS] {url}")
            
            # Try to get messages via frame evaluation
            try:
                # The WebSocket belongs to a frame - get it
                frames = ws.frames
                if frames:
                    frame = frames[0]
                    # Evaluate in the frame's context to see if there's a WS store
                    ws_info = frame.evaluate("""() => {
                        // Check if there are any WebSocket-related stores
                        return {
                            hasApp: !!document.getElementById('app'),
                            location: window.location.href
                        };
                    }""")
                    print(f"  [WS] Frame info: {ws_info}")
            except Exception as e:
                print(f"  [WS] Frame access: {e}")
        
        page.on("websocket", handle_websocket)
        
        # Navigate
        print("[2/5] Navigating...")
        page.goto("http://localhost:8000", wait_until="domcontentloaded", timeout=15000)
        
        # Wait
        print("[3/5] Waiting 15s...")
        time.sleep(15)
        
        # Check what WS connections we have
        print(f"\n[4/5] WS connections seen: {ws_connections}")
        
        # Now try to get the actual data by evaluating in page context
        # The Vue app stores WebSocket data in its reactive state
        print("[5/5] Extracting app state...")
        
        # Try to access Vue app's reactive state
        vue_state = page.evaluate("""() => {
            // Try to find the Vue app instance
            const app = document.getElementById('app');
            if (!app) return {error: 'no app'};
            
            // Try to get the host stream store
            // The app uses useHostStream(hostId) which returns {snapshot, connected, ...}
            // We can't directly access Vue refs, but we can check the DOM for rendered data
            
            // Check if there's a host detail view with process data
            const processCards = document.querySelectorAll('[class*="process"], [class*="Process"]');
            const hostCards = document.querySelectorAll('[class*="host"], [class*="Host"]');
            
            return {
                appFound: true,
                processCards: processCards.length,
                hostCards: hostCards.length,
                bodyText: document.body.innerText?.substring(0, 1000)
            };
        }""")
        print(f"    Vue state: {json.dumps(vue_state, ensure_ascii=False, indent=2)}")
        
        # Direct API calls
        print(f"\n\n{'='*70}")
        print("DIRECT API: /api/hosts/ai/overview (FULL)")
        print('='*70)
        
        overview = page.evaluate("""async () => {
            try {
                const r = await fetch('/api/hosts/ai/overview');
                return await r.json();
            } catch(e) { return {error: e.message}; }
        }""")
        print(json.dumps(overview, indent=2, ensure_ascii=False))
        
        # Console
        print(f"\n\n{'='*70}")
        print(f"CONSOLE ({len(console_msgs)} msgs)")
        print('='*70)
        for c in console_msgs[:20]:
            print(f"  {c}")
        
        browser.close()
    
    # Summary
    print(f"\n\n{'='*70}")
    print("SUMMARY")
    print('='*70)
    print(f"WS connections: {ws_connections}")
    print(f"\nKey findings from /api/hosts/ai/overview:")
    
    if overview and "host_metrics" in overview:
        hm = overview["host_metrics"]
        proc = hm.get("process")
        print(f"\n  host_metrics.process:")
        print(f"    Type: {type(proc).__name__}")
        if isinstance(proc, dict):
            print(f"    Keys: {list(proc.keys())}")
            print(f"    found: {proc.get('found')}")
            plist = proc.get("list", [])
            print(f"    list: [{len(plist)} items]")
            if plist:
                print(f"    First process keys: {list(plist[0].keys())}")
                print(f"    First process PID: {plist[0].get('pid')}")
                print(f"    First process model_path: {plist[0].get('model_path', '(none)')}")
        
        print(f"\n  host_metrics._model_paths: {hm.get('_model_paths', {})}")
        print(f"  host_metrics._mmproj_paths: {hm.get('_mmproj_paths', {})}")
        print(f"  host_metrics.service: {hm.get('service', {})}")
        
        llama = overview.get("llama", {})
        model = llama.get("model", {})
        print(f"\n  llama.model.name: {model.get('name', '(empty)')}")
        print(f"  llama.model.path: {model.get('path', '(empty)')}")
        print(f"  llama.online: {llama.get('online')}")

if __name__ == "__main__":
    run()
