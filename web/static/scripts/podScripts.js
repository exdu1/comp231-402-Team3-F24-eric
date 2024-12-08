document.addEventListener('DOMContentLoaded', () => {
  // Add environment variable entry
  document.querySelector('.add-env-var').addEventListener('click', (event) => {
      event.preventDefault();
      const envVarsDiv = document.querySelector('.env-vars');
      const newEntry = document.createElement('div');
      newEntry.classList.add('env-var-entry');
      newEntry.innerHTML = `
          <input type="text" name="env_key[]" placeholder="KEY">
          <input type="text" name="env_value[]" placeholder="VALUE">
          <button type="button" class="remove-env-var">-</button>
      `;
      envVarsDiv.appendChild(newEntry);

      // Add event listener to the remove button
      newEntry.querySelector('.remove-env-var').addEventListener('click', (event) => {
          event.preventDefault();
          newEntry.remove();
      });
  });

    // Add label entry
    document.querySelector('.add-label').addEventListener('click', (event) => {
        event.preventDefault();
        const labelsDiv = document.querySelector('.label-entry');
        const newEntry = document.createElement('div');
        newEntry.classList.add('label-entry');
        newEntry.innerHTML = `
          <input type="text" name="label_key[]" placeholder="KEY">
          <input type="text" name="label_value[]" placeholder="VALUE">
          <button type="button" class="remove-label">-</button>
      `;
        labelsDiv.appendChild(newEntry);

        // Add event listener to the remove button
        newEntry.querySelector('.remove-label').addEventListener('click', (event) => {
            event.preventDefault();
            newEntry.remove();
        });
    });

    // Add container entry
    document.querySelector('.add-container').addEventListener('click', (event) => {
        event.preventDefault();
        const containersDiv = document.getElementById('containers');
        const containerCount = containersDiv.children.length;
        const newEntry = document.createElement('div');
        newEntry.classList.add('container-entry');
        newEntry.innerHTML = `
            <input type="text" name="containers[${containerCount}][name]" placeholder="Container Name">
            <input type="text" name="containers[${containerCount}][image]" placeholder="Image">
            <input type="text" name="containers[${containerCount}][command][]" placeholder="Command">
            <div class="container-env-vars">
                <div class="container-env-var-entry">
                    <input type="text" name="containers[${containerCount}][env_key][]" placeholder="Env KEY">
                    <input type="text" name="containers[${containerCount}][env_value][]" placeholder="Env VALUE">
                    <button type="button" class="add-container-env-var">+</button>
                </div>
            </div>
            <button type="button" class="remove-container">-</button>
        `;
        containersDiv.appendChild(newEntry);

        // Add event listener to the remove button
        newEntry.querySelector('.remove-container').addEventListener('click', (event) => {
            event.preventDefault();
            newEntry.remove();
        });

        // Add event listener to the add-container-env-var button
        newEntry.querySelector('.add-container-env-var').addEventListener('click', (event) => {
            event.preventDefault();
            const containerEnvVarsDiv = newEntry.querySelector('.container-env-vars');
            const envVarCount = containerEnvVarsDiv.children.length;
            const newEnvVarEntry = document.createElement('div');
            newEnvVarEntry.classList.add('container-env-var-entry');
            newEnvVarEntry.innerHTML = `
                <input type="text" name="containers[${containerCount}][env_key][]" placeholder="Env KEY">
                <input type="text" name="containers[${containerCount}][env_value][]" placeholder="Env VALUE">
                <button type="button" class="remove-container-env-var">-</button>
            `;
            containerEnvVarsDiv.appendChild(newEnvVarEntry);

            // Add event listener to the remove button
            newEnvVarEntry.querySelector('.remove-container-env-var').addEventListener('click', (event) => {
                event.preventDefault();
                newEnvVarEntry.remove();
            });
        });
    });

  document.getElementById('podForm').addEventListener('submit', (event) => {
      event.preventDefault();

      const formData = new FormData(event.target);
      const data = Object.fromEntries(formData.entries());

      // Convert nested form data to JSON
      const jsonData = {
          id: data.id,
          name: data.name,
          containers: formData.getAll('containers[0][name]').map((name, i) => ({
              name,
              image: formData.getAll(`containers[${i}][image]`)[0],
              command: formData.getAll(`containers[${i}][command][]`),
              environment: Object.fromEntries(formData.getAll(`containers[${i}][env_key][]`).map((key, j) => [key, formData.getAll(`containers[${i}][env_value][]`)[j]]))
          })),
          resources: {
              cpu: parseInt(data['resources[cpu]']),
              memory: parseInt(data['resources[memory]'])
          },
          network: {
              hostNetwork: data['network[hostNetwork]'] === 'on',
              dns: {
                  nameservers: formData.getAll('network[dns][nameservers][]'),
                  searches: formData.getAll('network[dns][searches][]'),
                  options: formData.getAll('network[dns][options][]')
              },
              ports: [{
                  hostPort: parseInt(data['network[ports][0][hostPort]']),
                  containerPort: parseInt(data['network[ports][0][containerPort]']),
                  name: data['network[ports][0][name]'],
                  protocol: data['network[ports][0][protocol]']
              }]
          },
          volumes: [{
              name: data['volumes[0][name]'],
              hostPath: data['volumes[0][hostPath]']
          }],
          environment: Object.fromEntries(formData.getAll('env_key[]').map((key, i) => [key, formData.getAll('env_value[]')[i]])),
          restartPolicy: data.restartPolicy,
          labels: Object.fromEntries(formData.getAll('label_key[]').map((key, i) => [key, formData.getAll('label_value[]')[i]]))
      };

      fetch('/api/pods', {
          method: 'POST',
          headers: {
              'Content-Type': 'application/json'
          },
          body: JSON.stringify(jsonData)
      })
          .then(response => response.json())
          .then(data => {
              console.log('Success:', data);
              alert('Pod created successfully!');
          window.location.href = '/dashboard';
      })
          .catch((error) => {
              console.error('Error:', error);
          });
  });
});

function deletePod(podID) {
    if (confirm('Are you sure you want to delete this pod?')) {
        fetch(`/api/pods/${podID}`, {
            method: 'DELETE',
            headers: {
                'Content-Type': 'application/json'
            }
        })
        .then(response => {
            if (response.ok) {
                alert('Pod deleted successfully!');
                location.reload();
            } else {
                alert('Failed to delete pod.');
            }
        })
        .catch(error => {
            console.error('Error:', error);
            alert('An error occurred while deleting the pod.');
        });
    }
}