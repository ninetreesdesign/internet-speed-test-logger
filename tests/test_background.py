import csv
import importlib.util
import json
import math
import tempfile
import unittest
from datetime import datetime
from pathlib import Path
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('speedmon', Path(__file__).parents[1] / 'speedmon_bg.py')
app = importlib.util.module_from_spec(spec)
spec.loader.exec_module(app)

class BackgroundTests(unittest.TestCase):
    def summary(self, down=1000, up=35, ping=20):
        return {'Download': {'Throughput': {'Value': down, 'Unit': 'Mbit/s'},
                             'Latency': {'Value': ping, 'Unit': 'ms'}},
                'Upload': {'Throughput': {'Value': up, 'Unit': 'Mbit/s'}}, 'ServerFQDN': 'test.example'}

    def test_final_summary_and_units(self):
        output = json.dumps({'Key': 'measurement', 'Value': {'MeanClientMbps': 99999999}}) + '\n' + json.dumps(self.summary())
        self.assertEqual(app.parse_ndt7_summary(output), (1000, 35, 20, 'test.example'))
        s = self.summary(); s['Download']['Throughput']['Unit'] = 'bit/s'
        with self.assertRaises(RuntimeError): app.parse_ndt7_summary(json.dumps(s))
        with self.assertRaises(RuntimeError): app.parse_ndt7_summary('{"Key":"complete"}')

    def test_gates_missing_ping(self):
        for value in [996531233, float('nan'), float('inf'), -1, 0, 2001]:
            with self.assertRaises(RuntimeError): app.parse_ndt7_summary(json.dumps(self.summary(down=value)))
        for value in [8334302, 501]:
            with self.assertRaises(RuntimeError): app.parse_ndt7_summary(json.dumps(self.summary(up=value)))
        with self.assertRaises(RuntimeError): app.parse_ndt7_summary(json.dumps(self.summary(ping=2001)))
        self.assertEqual(app.parse_ndt7_summary(json.dumps(self.summary(ping=None)))[2], '')
        self.assertEqual(app.parse_ndt7_summary(json.dumps(self.summary(2000, 500, 2000)))[:3], (2000, 500, 2000))

    def test_platform_independent_timestamp(self):
        self.assertEqual(app.fmt_ts(datetime(2026, 9, 25, 0, 5)), '9/25/26 12:05 AM')
        self.assertEqual(app.fmt_ts(datetime(2026, 9, 25, 13, 5)), '9/25/26 1:05 PM')

    def test_old_csv_stays_intact_and_new_rows_share_epoch(self):
        with tempfile.TemporaryDirectory() as td, patch.object(app, 'LOG_DIR', Path(td)):
            old=app.speed_csv_path();old.write_text('timestamp,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result\nold,1,1,1,A,mac,eth,\n')
            before=old.read_bytes()
            app.append_speed_row('9/25/26 10:00 PM',down=1000,up=35,ping=20,epoch=1790398800.123)
            self.assertEqual(old.read_bytes(),before)
            with old.with_name(old.stem+'_v23.csv').open() as f: rows=list(csv.reader(f))
            self.assertEqual(rows[0],app.SPEED_HEADER);self.assertEqual(len(rows[1]),len(rows[0]));self.assertEqual(rows[1][1],'1790398800.123')
            app.append_url_rows('t',[('https://x/?a=1,2','timeout',10000)],False,'confirm')
            with app.url_csv_path().open() as f: rows=list(csv.reader(f))
            self.assertEqual(rows[1][-2:],['false','confirm'])

    def test_dropout_requires_both_rounds(self):
        down=[('a','down',0)]*3;up=[('a','up',1)]
        with patch.object(app,'check_witnesses',side_effect=[down,up]),patch.object(app.time,'sleep') as sleep:
            self.assertTrue(app.classify_connectivity()[0]);sleep.assert_called_once_with(10)
        with patch.object(app,'check_witnesses',side_effect=[down,down]),patch.object(app.time,'sleep'):
            self.assertFalse(app.classify_connectivity()[0])
        with patch.object(app,'check_witnesses',return_value=up) as probe,patch.object(app.time,'sleep') as sleep:
            self.assertTrue(app.classify_connectivity()[0]);probe.assert_called_once();sleep.assert_not_called()

if __name__ == '__main__': unittest.main()
