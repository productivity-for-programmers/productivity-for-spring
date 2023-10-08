import time 
import requests

while True:
    try:
        start = time.time()
        r = requests.get('http://localhost:9000/actuator/health');
        end = time.time()
        print(r.status_code, (end - start))
    except Exception as ex:
        print(str(ex))
    time.sleep(1)
