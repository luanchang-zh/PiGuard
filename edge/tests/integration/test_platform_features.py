"""Real authenticated Broker/Go/edge tests; no Raspberry Pi or Docker required.

Set PIGUARD_TEST_BACKEND_BIN and PIGUARD_TEST_MOSQUITTO_BIN to built executables.
Mosquitto's sibling mosquitto_passwd must also be available. Uses temporary
credentials, random loopback ports, isolated databases and owned subprocesses.
"""

import base64
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from dataclasses import replace

import paho.mqtt.client as mqtt
import pytest
import yaml

from piguard_edge.runtime.app import EdgeApp
from piguard_edge.runtime.clock import SystemClock
from piguard_edge.transport.mqtt import PahoMqtt

REPO = Path(__file__).resolve().parents[3]


def free_port():
    with socket.socket() as s:
        s.bind(('127.0.0.1',0))
        return s.getsockname()[1]


def test_platform_features_with_real_broker(example_config, tmp_path):
    backend = os.environ.get('PIGUARD_TEST_BACKEND_BIN')
    broker = os.environ.get('PIGUARD_TEST_MOSQUITTO_BIN')
    assert backend and broker, 'built backend and Mosquitto paths are required'
    port, api_port = free_port(), free_port()
    passwords = {key:secrets.token_hex(16) for key in ('platform','device','http','httpdevice')}
    env = dict(os.environ, PIGUARD_MQTT_USERNAME='platform', PIGUARD_MQTT_PASSWORD=passwords['platform'], PIGUARD_DEVICE_MQTT_PASSWORD=passwords['device'], PIGUARD_SEED_DEVICE_ID='car-001', PIGUARD_BROKER_HEALTH_USERNAME='health', PIGUARD_BROKER_HEALTH_PASSWORD=secrets.token_hex(16))
    env['PATH'] = str(Path(broker).parent.parent/'bin')+os.pathsep+env['PATH']
    subprocess.run(['sh',str(REPO/'go-backend/deploy/init-broker-auth.sh'),str(tmp_path/'auth')],env=env,check=True,capture_output=True)
    broker_config = tmp_path/'mosquitto.conf'
    broker_config.write_text(f'listener {port} 127.0.0.1\nallow_anonymous false\npassword_file {tmp_path}/auth/passwords\nacl_file {tmp_path}/auth/acl\n')
    env.update(PIGUARD_MQTT_BROKER=f'tcp://127.0.0.1:{port}', PIGUARD_HTTP_ADDR=f'127.0.0.1:{api_port}',PIGUARD_HTTP_PLATFORM_USERNAME='platform',PIGUARD_HTTP_PLATFORM_PASSWORD=passwords['http'], PIGUARD_HTTP_SEED_DEVICE_PASSWORD=passwords['httpdevice'],PIGUARD_DATABASE_SQLITE_PATH=str(tmp_path/'backend.db'),PIGUARD_STORAGE_SNAPSHOT_DIR=str(tmp_path/'snapshots'),GIN_MODE='release')
    config = replace(example_config,mqtt=replace(example_config.mqtt,port=port,username='car-001',password=passwords['device']),http=replace(example_config.http,base_url=f'http://127.0.0.1:{api_port}/api/v1',username='car-001',password=passwords['httpdevice']),storage=replace(example_config.storage,sqlite_path=tmp_path/'edge.db'),features=replace(example_config.features,frame_upload=True,remote_config=True))
    clock=SystemClock()
    app=EdgeApp.from_config(config,clock,PahoMqtt(config))
    app.start()  # Broker is deliberately absent on the first attempt.
    auth='Basic '+base64.b64encode(('platform:'+passwords['http']).encode()).decode()
    base=f'http://127.0.0.1:{api_port}'
    processes=[]
    logs=[]
    running=True
    def launch(args, name):
        log=(tmp_path/name).open('ab');logs.append(log)
        p=subprocess.Popen(args,env=env,stdout=log,stderr=subprocess.STDOUT)
        processes.append(p)
        return p
    def request(method,path,data=None):
        h={'Authorization':auth}
        if data is not None:h['Content-Type']='application/json'
        req=urllib.request.Request(base+path,data=None if data is None else json.dumps(data).encode(),headers=h,method=method)
        try:
            with urllib.request.urlopen(req,timeout=1) as r:return r.status,json.load(r)
        except urllib.error.HTTPError as exc:return exc.code,json.load(exc)
        except (OSError,urllib.error.URLError):return 0,{}
    def wait(condition,timeout=15):
        end=time.monotonic()+timeout
        while time.monotonic()<end:
            if running:app.step()
            value=condition()
            if value:return value
            time.sleep(.02)
        pytest.fail('condition timed out; '+ '; '.join((tmp_path/x).read_text()[-1500:] for x in ('backend.log','broker.log') if (tmp_path/x).exists()))
    def state():return request('GET','/api/v1/devices/car-001/state')[1].get('data',{})
    def command(kind,params):
        status,body=request('POST','/api/v1/devices/car-001/commands',{'type':kind,'params':params});assert status==202,body
        cid=body['data']['command_id']
        result=wait(lambda: (c if (c:=request('GET','/api/v1/commands/'+cid)[1]['data'])['status'] in ('success','failed','timeout') else None))
        assert result['status']=='success',result
        return result
    try:
        for _ in range(5): app.step();time.sleep(.04)
        assert app._sampler.state.speed.value==30
        bp=launch([broker,'-c',str(broker_config)],'broker.log')
        time.sleep(.3)
        launch([backend,'-config',str(REPO/'go-backend/config.yaml')],'backend.log')
        wait(lambda:request('GET','/health')[0]==200)
        wait(lambda:state().get('online'))
        wait(lambda:request('GET','/api/v1/devices/car-001/config')[1]['data']['status']=='success')
        assert app._config.device.config_version==1
        wait(lambda:request('GET','/api/v1/devices/car-001/frame')[0]==200)
        shot=command('camera.snapshot',{'quality':80})
        snapshot=shot['result']['snapshot_id']
        with urllib.request.urlopen(urllib.request.Request(base+'/api/v1/snapshots/'+snapshot+'/content',headers={'Authorization':auth}),timeout=2) as r:
            assert r.headers['Content-Type']=='image/jpeg' and r.read()[:2]==b'\xff\xd8'
        client=mqtt.Client(mqtt.CallbackAPIVersion.VERSION2,client_id='feature-test-platform')
        client.username_pw_set('platform',passwords['platform']);client.connect('127.0.0.1',port);client.loop_start()
        try:
            wait(client.is_connected)
            payload={key:shot[key] for key in ('command_id','device_id','type','params','issued_at','expires_at')};payload['schema_version']=1
            client.publish('car/car-001/commands',json.dumps(payload),qos=1).wait_for_publish(3)
            deadline=time.monotonic()+.5
            while time.monotonic()<deadline:app.step();time.sleep(.02)
            assert request('GET','/api/v1/commands/'+shot['command_id'])[1]['data']['executed_at']==shot['executed_at']
        finally:client.disconnect();client.loop_stop()
        command('buzzer.test',{'duration_ms':300})
        status,body=request('PUT','/api/v1/devices/car-001/config',{'expected_version':1,'rules':{'obstacle':{'warning_distance_m':60,'danger_distance_m':55}}})
        assert status==202,body
        wait(lambda:request('GET','/api/v1/devices/car-001/config')[1]['data']['status']=='success')
        wait(lambda:state().get('risk_level')=='danger')
        assert app._config.device.config_version==2
        wait(lambda:app._store.counts()[0]>0 and app._store.counts()[1]==0)
        before=request('GET','/api/v1/devices/car-001/events')[1]['data']['items']
        bp.terminate();bp.wait(5)
        wait(lambda:not state().get('online'))
        launch([broker,'-c',str(broker_config)],'broker.log')
        wait(lambda:state().get('online'),25)
        wait(lambda:request('GET','/api/v1/devices/car-001/config')[1]['data']['status']=='success')
        after=request('GET','/api/v1/devices/car-001/events')[1]['data']['items']
        assert {e['event_id'] for e in before}=={e['event_id'] for e in after}
        app.stop();running=False
        wait(lambda:not state().get('online'))
        # Exercise the installed CLI and actual SIGTERM handling after restoring v2.
        raw=yaml.safe_load(example_config.source_path.read_text())
        raw['mqtt'].update(host='127.0.0.1',port=port,username='car-001',password=passwords['device'])
        raw['http'].update(base_url=config.http.base_url,username='car-001',password=passwords['httpdevice'])
        raw['features'].update(frame_upload=True,remote_config=True)
        raw['storage']['sqlite_path']=str(tmp_path/'edge.db')
        path=tmp_path/'edge.yaml';path.write_text(yaml.safe_dump(raw));path.chmod(0o600)
        cli=launch([sys.executable,'-m','piguard_edge','--config',str(path)],'edge.log')
        wait(lambda:state().get('online'))
        cli.terminate();assert cli.wait(5)==0
        wait(lambda:not state().get('online'))
    finally:
        if running:app.stop()
        for p in reversed(processes):
            if p.poll() is None:
                p.terminate()
                try:p.wait(5)
                except subprocess.TimeoutExpired:p.kill();p.wait()
        for log in logs:log.close()
