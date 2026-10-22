from types import SimpleNamespace

import pytest

from piguard_edge.drivers import factory
from piguard_edge.runtime.app import EdgeApp
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.transport.mqtt import FakeMqtt


def test_factory_closes_previous_sources_when_later_driver_fails(monkeypatch):
    closed = []
    monkeypatch.setattr(factory, 'MockDistanceSensor', lambda *args: SimpleNamespace(close=lambda: closed.append('distance')))
    with pytest.raises(factory.DriverUnavailable):
        factory.build_sources(ScenarioEngine('normal_drive', 1), distance_scale=100,
                              distance_driver='mock', camera_driver='missing', gyro_driver='mock',
                              temperature_driver='mock', speed_driver='scenario')
    assert closed == ['distance']


def test_mock_factory_never_reads_hardware_configuration(example_config):
    class ForbiddenHardware:
        def __getattr__(self, name): raise AssertionError('mock driver accessed hardware')
    sources = factory.build_sources(ScenarioEngine('normal_drive', 1), distance_scale=100,
                                     distance_driver='mock', camera_driver='synthetic', gyro_driver='mock',
                                     temperature_driver='mock', speed_driver='scenario', hardware=ForbiddenHardware())
    assert sources.distance.read(FakeClock()).source == 'mock'


def test_sampler_closes_every_source_even_if_one_close_fails(example_config):
    sampler = Sampler.from_config(example_config, ScenarioEngine('normal_drive', 1), FakeClock())
    closed = []
    def close(name):
        closed.append(name)
        if name == 'camera': raise OSError('already disconnected')
    for name in ('camera', 'distance', 'gyro', 'temperature', 'speed'):
        setattr(sampler.sources, name, SimpleNamespace(close=lambda name=name: close(name)))
    with pytest.raises(OSError): sampler.close()
    assert closed == ['camera', 'distance', 'gyro', 'temperature', 'speed']


def test_app_constructor_failure_closes_sampler_and_store(example_config, monkeypatch):
    from piguard_edge.runtime import app as module
    closed = []
    monkeypatch.setattr(module.Sampler, 'from_config', lambda *args: SimpleNamespace(close=lambda: closed.append('sampler')))
    monkeypatch.setattr(module, 'EventStore', lambda *args: SimpleNamespace(close=lambda: closed.append('store')))
    def broken_gpio(*args): raise OSError('GPIO busy')
    monkeypatch.setattr(module.ActuatorManager, 'from_config', broken_gpio)
    with pytest.raises(OSError, match='GPIO busy'):
        EdgeApp.from_config(example_config, FakeClock(), FakeMqtt())
    assert closed == ['store', 'sampler']


def test_app_shutdown_closes_all_resources_after_disconnect_error(example_config):
    from test_uplink import _app
    app, _, transport = _app(example_config)
    closed = []
    def disconnect(): raise RuntimeError('transport close failed')
    transport.disconnect = disconnect
    app._actuators.close = lambda: closed.append('actuator')
    app._sampler.close = lambda: closed.append('sampler')
    close_store = app._store.close
    def store():
        close_store()
        closed.append('store')
    app._store.close = store
    with pytest.raises(RuntimeError, match='transport close failed'): app.stop()
    assert closed == ['actuator', 'sampler', 'store']
