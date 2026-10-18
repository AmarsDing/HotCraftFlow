import fs from 'fs';
const p = process.argv[2];
let s = fs.readFileSync(p, 'utf8');
const bad = `		if retryRepo != nil {
			if retryErr := retryRepo.EnqueueRetry(ctx, topic, eventMap, err.Error()); retryErr != nil {
			logger.Errorf("enqueue nats retry for %s also failed: %s", topic, retryErr.Error())
		}
	}
}`;
const good = `		if retryRepo != nil {
			if retryErr := retryRepo.EnqueueRetry(ctx, topic, eventMap, err.Error()); retryErr != nil {
				logger.Errorf("enqueue nats retry for %s also failed: %s", topic, retryErr.Error())
			}
		}
	}
}`;
if (!s.includes(bad)) {
  // try CRLF variant
  const bad2 = bad.replace(/\n/g, '\r\n');
  const good2 = good.replace(/\n/g, '\r\n');
  if (s.includes(bad2)) { s = s.replace(bad2, good2); console.log('fixed crlf'); }
  else { console.log('pattern not found'); process.exit(1); }
} else {
  s = s.replace(bad, good);
  console.log('fixed lf');
}
fs.writeFileSync(p, s);
